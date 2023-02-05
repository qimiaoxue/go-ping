package main

import (
	"fmt"
	"net"
)

type Pinger struct {
	ipaddr *net.IPAddr
	addr   string
}

func (p *Pinger) SetAddr(addr string) error {
	ipaddr, err := net.ResolveIPAddr("ip4:icmp", addr)
	if err != nil {
		return err
	}
	p.addr = addr
	p.ipaddr = ipaddr
	return nil
}

func (p *Pinger) Addr() string {
	return p.addr
}

func (p *Pinger) SetIPAddr(ipaddr *net.IPAddr) {
	p.ipaddr = ipaddr
	p.addr = ipaddr.String()
}

func (p *Pinger) IPAddr() *net.IPAddr {
	return p.ipaddr
}
func main() {
	pinger := &Pinger{}
	err := pinger.SetAddr("www.google.com")
	if err != nil {
		fmt.Printf("set addr errr: %s\n", err)
		return
	}
	fmt.Printf("PING %s (%s)\n", pinger.Addr(), pinger.IPAddr())
}
