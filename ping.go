package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
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
	Checksum   uint16
	Identifier uint16
	Sequence   uint16
}

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

func parseResp(resp []byte) (*reply, error) {
	if length := binary.LittleEndian.Uint16(resp[2:]); length != uint16(len(resp[IpHeaderLength:])) &&
		length != IcmpHeaderLength+PayloadLength {
		fmt.Println(length)
		return nil, errors.New("bad response")
	}
	r := reply{
		TTL:        resp[8],
		Type:       resp[IpHeaderLength],
		Checksum:   binary.BigEndian.Uint16(resp[IpHeaderLength+2:]),
		Identifier: binary.BigEndian.Uint16(resp[IpHeaderLength+4:]),
		Sequence:   binary.BigEndian.Uint16(resp[IpHeaderLength+6:]),
	}
	if checksum(resp[IpHeaderLength:]) != r.Checksum {
		return nil, errors.New("incorrect checksum")
	}
	return &r, nil
}

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

func (p *Pinger) Ping() {
	// connect to host
	conn, err := net.DialTimeout("ip4:icmp", p.Ip, time.Second*time.Duration(p.Timeout))
	if err != nil {
		p.ch <- err.Error()
		return
	}
	p.ch <- fmt.Sprintf("PING %s (%s): %d data bytes", p.Host, p.Ip, PayloadLength)
	writer := bufio.NewWriter(conn)
	msg, identifier := createMessage()
	p.min = math.MaxFloat32
	var times []float32
	for i := 0; i < p.Repeat; i++ {
		startTime := time.Now()
		err = conn.SetDeadline(startTime.Add(time.Second * 5))
		if err != nil {
			p.ch <- err.Error()
			continue
		}
		m, _ := msg()
		_, err = writer.Write(m)
		if err != nil {
			p.ch <- err.Error()
			continue
		}
		_ = writer.Flush()
		p.transmitted++
		res := make([]byte, IpHeaderLength+IcmpHeaderLength+PayloadLength)
		_, err = conn.Read(res)
		//fmt.Printf("time=%d\n", time.Since(startTime).Microseconds())
		//p.ch <- fmt.Sprintf("time=%d\n", time.Since(startTime).Microseconds())
		//p.ch <- hex.EncodeToString(res)
		if err != nil {
			p.ch <- err.Error()
			continue
		}
		p.received++
		r, err := parseResp(res)
		if err != nil {
			p.ch <- err.Error()
			continue
		}
		t := float32(time.Since(startTime).Microseconds()) / 1000.0
		if t > p.max {
			p.max = t
		}
		if t < p.min {
			p.min = t
		}
		times = append(times, t)
		p.ch <- fmt.Sprintf("%d bytes from %s: icmp_seq=%d ttl=%d time=%.3f ms",
			len(res)-IpHeaderLength,
			conn.RemoteAddr(),
			r.Sequence,
			r.TTL,
			t,
		)
		time.Sleep(time.Second)
	}
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
