package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

var usage = `
Usage:
	ping host
	
Example:
# ping google continuely
ping www.google.com

# ping google 5 times
ping -c 5 www.google.com

#ping google 5 times at 500ms intervals
ping -c 5 -i 500ms www.google.com
`

func NewPinger(host string) (*Pinger, error) {
	p := &Pinger{}
	err := p.SetAddr(host)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type Pinger struct {
	ipaddr     *net.IPAddr
	addr       string
	sourceAddr string
	Count      int
	Interval   time.Duration
	PacketRecv int
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

func (p *Pinger) Run() {
	conn, err := icmp.ListenPacket("ip4:icmp", p.sourceAddr)

	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}
	//wg := sync.WaitGroup{}

	interval := time.NewTicker(p.Interval)

	go func() {
		for {
			bytesGot := make([]byte, 512)
			n, _, err := conn.ReadFrom(bytesGot)

			if err != nil {
				return
			}
			//fmt.Printf("bytes receiverd %s, %d\n", bytesGot, n)

			rm, err := icmp.ParseMessage(1, bytesGot[:n])
			if err != nil {
				return
			}
			//fmt.Printf("bytes received: %v, %d\n", rm, n)

			pkt := rm.Body.(*icmp.Echo)
			Rtt := time.Since(bytesToTime(pkt.Data[:8]))
			fmt.Printf("RTT is %s\n", Rtt)
			p.PacketRecv++
		}
	}()

	_ = p.sendICMP(conn)

	for {
		//fmt.Println("time.interval: ", time.Now())
		select {
		case <-interval.C:
			_ = p.sendICMP(conn)
		default:
			if p.Count > 0 && p.PacketRecv >= p.Count {
				return
			}
		}
	}
}

func (p *Pinger) sendICMP(conn *icmp.PacketConn) error {
	bytes, err := (&icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{
			ID:   rand.Intn(65535),
			Seq:  1,
			Data: timeToBytes(time.Now()),
		},
	}).Marshal(nil)

	if err != nil {
		time.Sleep(p.Interval)
		return err
	}

	_, err = conn.WriteTo(bytes, p.ipaddr)
	if err != nil {
		time.Sleep(p.Interval)
		return err
	}
	return err
}

func main() {
	flag.Usage = func() {
		fmt.Printf(usage)
	}

	count := flag.Int("c", -1, "")
	interval := flag.Duration("i", time.Second, "")

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		return
	}

	host := flag.Arg(0)
	pinger, err := NewPinger(host)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
		return
	}
	fmt.Printf("PING %s (%s)\n", pinger.Addr(), pinger.IPAddr())
	pinger.Count = *count
	pinger.Interval = *interval
	pinger.Run()
}

func bytesToTime(b []byte) time.Time {
	var nsec int64
	for i := uint8(0); i < 8; i++ {
		nsec += int64(b[i]) << ((7 - i) * 8)
	}
	return time.Unix(nsec/1000000000, nsec%1000000000)
}

func timeToBytes(t time.Time) []byte {
	nsec := t.UnixNano()
	b := make([]byte, 8)
	for i := uint8(0); i < 8; i++ {
		b[i] = byte((nsec >> ((7 - i) * 8)) & 0xff)
	}
	return b
}
