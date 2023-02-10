package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
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

$ping google for 10 seconds
ping -t 10s www.google.com
`

const UDP = "udp"

func NewPinger(host string) (*Pinger, error) {
	p := &Pinger{stat: &Stat{}, network: UDP}
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
	Timeout    time.Duration
	sequence   int
	stat       *Stat
	network    string
}

type Stat struct {
	PacketRecv int
	PacketSent int
	PacketLoss float64
	Rtts       []time.Duration
	MinRtt     time.Duration
	MaxRtt     time.Duration
	AvgRtt     time.Duration
	SumRtt     time.Duration
	SumSqrtRtt time.Duration
	StdDevRtt  time.Duration
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

func (p *Pinger) finish() {
	stat := p.Stat()
	fmt.Printf("\n--- %s ping statistics ---\n", p.addr)
	fmt.Printf("%d packets transmitted, %d packets received, %v%% packet loss\n", stat.PacketSent, stat.PacketRecv, stat.PacketLoss)
	fmt.Printf("round-trip min/avg/max/stddev = %v/%v/%v/%v\n", stat.MinRtt, stat.AvgRtt, stat.MaxRtt, stat.StdDevRtt)
}

func (p *Pinger) Stat() *Stat {
	p.stat.PacketLoss = float64(p.stat.PacketSent-p.stat.PacketRecv) / float64(p.stat.PacketSent) * 100
	if len(p.stat.Rtts) > 0 {
		p.stat.StdDevRtt = time.Duration(math.Sqrt(float64(p.stat.SumSqrtRtt / time.Duration(len(p.stat.Rtts)))))
	}
	return p.stat
}

func (p *Pinger) Run() {
	ctxtimeout, cancel := context.WithTimeout(context.Background(), p.Timeout)
	defer cancel()
	defer p.finish()

	proto := "ip4:icmp"
	if p.network == "udp" {
		proto = "udp4"
	}

	conn, err := icmp.ListenPacket(proto, p.sourceAddr)

	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}
	//wg := sync.WaitGroup{}
	closed := make(chan interface{})

	interval := time.NewTicker(p.Interval)
	//timeout := time.NewTicker(p.Timeout)

	go func() {
		for {
			bytesGot := make([]byte, 512)
			n, _, err := conn.ReadFrom(bytesGot)

			if err != nil {
				return
			}
			//fmt.Printf("bytes receiverd %s, %d\n", bytesGot, n)
			bytes := ipv4PayLoad(bytesGot)
			rm, err := icmp.ParseMessage(1, bytes[:n])
			if err != nil {
				return
			}
			//fmt.Printf("bytes received: %v, %d\n", rm, n)

			pkt := rm.Body.(*icmp.Echo)
			Rtt := time.Since(bytesToTime(pkt.Data[:8]))
			//fmt.Printf("RTT is %s\n", Rtt)
			fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n", n, p.ipaddr, pkt.Seq, Rtt)
			p.stat.PacketRecv += 1
			p.stat.Rtts = append(p.stat.Rtts, Rtt)
			if Rtt > p.stat.MaxRtt {
				p.stat.MaxRtt = Rtt
			}

			if p.stat.MinRtt == 0 || Rtt < p.stat.MinRtt {
				p.stat.MinRtt = Rtt
			}
			p.stat.SumRtt += Rtt
			p.stat.AvgRtt = p.stat.SumRtt / time.Duration(len(p.stat.Rtts))
			p.stat.SumSqrtRtt += (Rtt - p.stat.AvgRtt) * (Rtt - p.stat.AvgRtt)
			p.PacketRecv++
			if p.PacketRecv == p.Count {
				close(closed)
				return
			}
		}
	}()

	_ = p.sendICMP(conn)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)

	for {
		//fmt.Println("time.interval: ", time.Now())
		select {
		//case <-timeout.C:
		//return
		case <-c:
			return
		case <-ctxtimeout.Done():
			return
		case <-interval.C:
			_ = p.sendICMP(conn)
		case <-closed:
			return
		}
	}
}

func (p *Pinger) sendICMP(conn *icmp.PacketConn) error {
	bytes, err := (&icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{
			ID:   rand.Intn(65535),
			Seq:  p.sequence,
			Data: timeToBytes(time.Now()),
		},
	}).Marshal(nil)

	if err != nil {
		time.Sleep(p.Interval)
		return err
	}

	var dst net.Addr = p.ipaddr
	if p.network == "udp" {
		dst = &net.UDPAddr{IP: p.ipaddr.IP, Zone: p.ipaddr.Zone}
	}
	for {
		if _, err = conn.WriteTo(bytes, dst); err != nil {
			if neterr, ok := err.(*net.OpError); ok {
				if neterr.Err == syscall.ENOBUFS {
					continue
				}
			}
		}
		p.sequence += 1
		p.stat.PacketSent += 1
		break
	}
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Printf(usage)
	}

	count := flag.Int("c", -1, "")
	interval := flag.Duration("i", time.Second, "")
	timeout := flag.Duration("t", time.Second*100000, "")
	privileged := flag.Bool("privileged", false, "")

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
	pinger.Timeout = *timeout
	pinger.setPrivileged(*privileged)
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

func ipv4PayLoad(b []byte) []byte {
	if len(b) < ipv4.HeaderLen {
		return b
	}
	hdrlen := int(b[0]&0x0f) << 2
	return b[hdrlen:]
}

func (p *Pinger) setPrivileged(privileged bool) {
	if privileged {
		p.network = "ip"
	} else {
		p.network = "udp"
	}
}
