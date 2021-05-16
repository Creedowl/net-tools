package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"syscall"
	"time"
)

const (
	IcmpHeaderLength = 8
	PayloadLength    = 48
	IpHeaderLength   = 20
)

type Pinger struct {
	Host        string
	Ip          string
	Repeat      int
	Timeout     int
	ch          chan string
	transmitted int
	received    int
	min         float32
	avg         float32
	max         float32
	stddev      float32
}

type reply struct {
	TTL        uint8
	Type       uint8
	Code       uint8
	Checksum   uint16
	Identifier uint16
	Sequence   uint16
	Size       int
	Ip         string
}

// NewPinger creates a new pinger instance
func NewPinger(host string, repeat, timeout int, ch chan string) (*Pinger, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("failed to resolve host")
	}
	ip := ""
	for _, v := range ips {
		if i := v.To4(); i != nil {
			ip = i.String()
		}
	}
	if ip == "" {
		return nil, errors.New("failed to resolve ipv4 host")
	}
	return &Pinger{
		Host:    host,
		Ip:      ip,
		Repeat:  repeat,
		Timeout: timeout,
		ch:      ch,
	}, nil
}

// create a Ping ICMP datagram, return a function to generate datagram with increasing identifier
func createMessage() (func() ([]byte, uint16), uint16) {
	msg := [IcmpHeaderLength + PayloadLength]byte{
		0x8, // type
		0x0, // code
		0x0, // checksum, 16 bits
		0x0,
		0x0, // identifier, 16 bits, random for every ping process
		0x0,
		0x0, // sequence number, 16 bits, an increasing number within the process
		0x0,
		// the rest part is payload
		0xde,
		0xed,
		0xbe,
		0xef,
	}

	rand.Seed(time.Now().Unix())

	// generate random identifier
	identifier := uint16(rand.Uint32())
	binary.BigEndian.PutUint16(msg[4:], identifier)

	var sequence uint16 = 0

	return func() ([]byte, uint16) {
		binary.BigEndian.PutUint16(msg[6:], sequence)
		sequence++
		binary.BigEndian.PutUint16(msg[2:], checksum(msg[:]))
		return msg[:], sequence
	}, identifier
}

// calculate the checksum
func checksum(msg []byte) uint16 {
	// clear origin checksum
	binary.BigEndian.PutUint16(msg[2:], 0x0)

	length := len(msg)
	var sum uint64 = 0
	for i := 0; i < length; i += 2 {
		sum += uint64(binary.BigEndian.Uint16(msg[i:]))
	}
	if length%2 != 0 {
		sum += uint64(binary.BigEndian.Uint16(msg[length-1:]))
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return uint16(^sum)
}

// parse the ICMP response
func parseResp(resp []byte, identifier uint16) (*reply, error) {
	//fmt.Println(hex.EncodeToString(resp))
	length := binary.LittleEndian.Uint16(resp[2:])
	// mismatch datagram length
	if length != uint16(len(resp[IpHeaderLength:])) {
		return nil, errors.New("bad response")
	}
	id := binary.BigEndian.Uint16(resp[IpHeaderLength+4:])
	sum := binary.BigEndian.Uint16(resp[IpHeaderLength+2:])
	if id != identifier {
		return nil, errors.New("identifier does not match")
	}
	if checksum(resp[IpHeaderLength:]) != sum {
		return nil, errors.New("incorrect checksum")
	}
	r := reply{
		TTL:        resp[8],
		Type:       resp[IpHeaderLength],
		Code:       resp[IpHeaderLength+1],
		Checksum:   sum,
		Identifier: id,
		Sequence:   binary.BigEndian.Uint16(resp[IpHeaderLength+6:]),
		Size:       int(length),
	}
	return &r, nil
}

// count the results
func statistic(times []float32) (avg, stddev float32) {
	var sum float32 = 0
	for _, v := range times {
		sum += v
	}
	avg = sum / float32(len(times))
	sum = 0
	for _, v := range times {
		sum += float32(math.Pow(float64(v-avg), 2))
	}
	stddev = float32(math.Sqrt(float64(sum / float32(len(times)))))
	return
}

// receive inbound datagram
func (p *Pinger) listen(conn *Socket, identifier uint16, sequence chan int) chan *reply {
	ch := make(chan *reply)
	res := make([]byte, 128)
	go func() {
		seq := <-sequence
		for {
			size, from, err := conn.Read(res)
			// read response timeout
			if err != nil {
				p.ch <- fmt.Sprintf("Request timeout for icmp_seq %d", seq)
				ch <- nil
				// read next response
				s, ok := <-sequence
				if !ok {
					break
				}
				seq = s
				continue
			}
			inet4 := from.(*syscall.SockaddrInet4)
			r, err := parseResp(res[:size], identifier)
			// mismatch ICMP response, sent from other processes
			if err != nil {
				continue
			}
			r.Ip = net.IP(inet4.Addr[:]).String()
			ch <- r
			// wait the next response
			s, ok := <-sequence
			if !ok {
				break
			}
			seq = s
		}
	}()
	return ch
}

// Ping starts pinging
func (p *Pinger) Ping() {
	// connect to host
	conn, err := NewSocket(net.ParseIP(p.Ip).To4(), time.Duration(p.Timeout)*time.Second)
	if err != nil {
		p.ch <- err.Error()
		return
	}
	p.ch <- fmt.Sprintf("PING %s (%s): %d data bytes", p.Host, p.Ip, PayloadLength)
	msg, identifier := createMessage()
	p.min = math.MaxFloat32
	var times []float32

	// send current sequence to listener
	sequence := make(chan int)
	// receive response
	ch := p.listen(conn, identifier, sequence)
	for i := 0; i < p.Repeat; i++ {
		startTime := time.Now()
		m, _ := msg()
		err = conn.Send(m)
		if err != nil {
			p.ch <- err.Error()
			continue
		}
		sequence <- i
		p.transmitted++

		// blocking until receive valid response or error
		r := <-ch
		if r == nil {
			continue
		}
		p.received++

		t := float32(time.Since(startTime).Microseconds()) / 1000.0
		if t > p.max {
			p.max = t
		}
		if t < p.min {
			p.min = t
		}
		times = append(times, t)

		p.ch <- fmt.Sprintf("%d bytes from %s: icmp_seq=%d ttl=%d time=%.3f ms",
			r.Size,
			r.Ip,
			r.Sequence,
			r.TTL,
			t,
		)
		time.Sleep(time.Second)
	}
	close(ch)
	// show statistic result
	p.ch <- fmt.Sprintf("--- %s ping statistic ---", p.Host)
	p.ch <- fmt.Sprintf("identifier: %d", identifier)
	p.ch <- fmt.Sprintf("%d packets transmitted, %d packets received, %.1f%% packet loss",
		p.transmitted,
		p.received,
		float32(p.transmitted-p.received)/float32(p.transmitted)*100,
	)
	if p.min != math.MaxFloat32 {
		p.avg, p.stddev = statistic(times)
		p.ch <- fmt.Sprintf("round-trip min/avg/max/stddev = %.3f/%.3f/%.3f/%.3f ms",
			p.min,
			p.avg,
			p.max,
			p.stddev,
		)
	}
	close(p.ch)
	return
}
