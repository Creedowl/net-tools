package main

import (
	"net"
	"syscall"
	"time"
)

type Socket struct {
	fd   int
	addr *syscall.SockaddrInet4
}

func NewSocket(ip net.IP, timeout time.Duration) (*Socket, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
	if err != nil {
		return nil, err
	}
	tv := syscall.NsecToTimeval(timeout.Nanoseconds())
	// set socket send timeout
	err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)
	if err != nil {
		return nil, err
	}
	// set socket receive timeout
	err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
	if err != nil {
		return nil, err
	}
	// convert slice to array
	var addr [4]byte
	copy(addr[:], ip)

	return &Socket{
		fd: fd,
		addr: &syscall.SockaddrInet4{
			Addr: addr,
		},
	}, nil
}

func (s *Socket) Close() error {
	return syscall.Close(s.fd)
}

func (s *Socket) Send(data []byte) error {
	return syscall.Sendto(s.fd, data, 0, s.addr)
}

func (s *Socket) Read(buf []byte) (int, syscall.Sockaddr, error) {
	return syscall.Recvfrom(s.fd, buf, 0)
}

func (s *Socket) Bind() error {
	return syscall.Bind(s.fd, s.addr)
}

func (s *Socket) SetTTL(ttl int) error {
	return syscall.SetsockoptInt(s.fd, 0, syscall.IP_TTL, ttl)
}
