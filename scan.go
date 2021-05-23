package main

import (
	"fmt"
	"net"
	"sync"
)

type Scanner struct {
	m     sync.Mutex
	IP    net.IP
	IPNet *net.IPNet
	ch    chan string
}

func NewScanner(cidr string, ch chan string) (*Scanner, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	return &Scanner{
		IP:    ip,
		IPNet: ipnet,
		ch:    ch,
	}, nil
}

func nextIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func (s *Scanner) sendMsg(msg string) {
	if s.ch == nil {
		return
	}
	s.ch <- msg
}

func (s *Scanner) Scan() []net.IP {
	wg := sync.WaitGroup{}
	var result []net.IP
	s.sendMsg("start scanning")
	for ip := s.IP.Mask(s.IPNet.Mask); s.IPNet.Contains(ip); nextIP(ip) {
		_ip := make(net.IP, len(ip))
		copy(_ip, ip)
		wg.Add(1)
		go func() {
			defer wg.Done()
			//s.sendMsg(fmt.Sprintf("start checking ip: %s", _ip.String()))
			pinger, err := NewPinger(_ip.String(), 2, 2, nil)
			if err != nil {
				s.sendMsg(err.Error())
				return
			}
			if pinger.Ping() {
				s.sendMsg(fmt.Sprintf("ip %s is reachable", _ip))
				s.m.Lock()
				result = append(result, _ip)
				s.m.Unlock()
			} else {
				//s.sendMsg(fmt.Sprintf("ip %s is unreachable", _ip))
			}
		}()
	}
	wg.Wait()
	s.sendMsg("finish scanning")
	return result
}
