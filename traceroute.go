package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

type Tracer struct {
	Host string
	IP   string
	ch   chan string
}

func NewTracer(host string, ch chan string) (*Tracer, error) {
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
	return &Tracer{
		Host: host,
		IP:   ip,
		ch:   ch,
	}, nil
}

func (t *Tracer) sendMsg(msg string) {
	if t.ch == nil {
		return
	}
	t.ch <- msg
}

func (t *Tracer) parseResp(resp []byte, identifier uint16) (*reply, error) {
	length := binary.LittleEndian.Uint16(resp[2:])
	// mismatch datagram length
	if length != uint16(len(resp[IpHeaderLength:])) {
		return nil, errors.New("bad response")
	}
	type_ := resp[IpHeaderLength]
	code := resp[IpHeaderLength+1]
	var id uint16
	if type_ == 11 {
		// time exceeded
		id = binary.BigEndian.Uint16(resp[IpHeaderLength*2+4+8:])
	} else if type_ == 0 {
		// echo reply
		id = binary.BigEndian.Uint16(resp[IpHeaderLength+4:])
	} else {
		return nil, errors.New(fmt.Sprintf("mismatch type %d", type_))
	}
	if id != identifier {
		//fmt.Println(id)
		//t.sendMsg(strconv.Itoa(int(id)))
		return nil, errors.New("identifier does not match")
	}
	r := reply{
		TTL:        resp[8],
		Type:       type_,
		Code:       code,
		Identifier: id,
		Size:       int(length),
	}
	return &r, nil
}

// receive inbound datagram
func (t *Tracer) listen(conn *Socket, identifier uint16, sequence chan int) chan *reply {
	ch := make(chan *reply)
	res := make([]byte, 128)
	go func() {
		seq := <-sequence
		for {
			size, from, err := conn.Read(res)
			if err != nil {
				t.sendMsg(fmt.Sprintf("%d *", seq))
				ch <- nil
			} else {
				inet4 := from.(*syscall.SockaddrInet4)
				r, err := t.parseResp(res[:size], identifier)
				if err != nil {
					continue
				}
				r.Ip = net.IP(inet4.Addr[:]).String()
				ch <- r
			}
			s, ok := <-sequence
			if !ok {
				break
			}
			seq = s
		}
		err := conn.Close()
		if err != nil {
			t.sendMsg(err.Error())
		}
	}()
	return ch
}

func (t *Tracer) Trace() {
	conn, err := NewSocket(net.ParseIP(t.IP).To4(), 5*time.Second)
	if err != nil {
		t.sendMsg(err.Error())
		return
	}
	t.sendMsg(fmt.Sprintf("traceroute to %s (%s), 64 hops max", t.Host, t.IP))
	msg, identifier := createMessage()
	sequence := make(chan int)
	ch := t.listen(conn, identifier, sequence)
	for i := 1; i <= 64; i++ {
		err := conn.SetTTL(i)
		if err != nil {
			t.sendMsg(err.Error())
			continue
		}
		m, _ := msg()
		startTime := time.Now()
		err = conn.Send(m)
		if err != nil {
			t.sendMsg(err.Error())
			continue
		}
		sequence <- i
		r := <-ch
		if r == nil {
			continue
		}
		ti := float32(time.Since(startTime).Microseconds()) / 1000.0
		t.sendMsg(fmt.Sprintf("%d %s  %.3fms", i, r.Ip, ti))
		if r.Ip == t.IP {
			break
		}
		time.Sleep(time.Second)
	}

	err = conn.Close()
	if err != nil {
		t.sendMsg(err.Error())
	}
}
