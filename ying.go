package ying

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const UDP = "udp"

func NewPinger(host string) (*Pinger, error) {
	p := &Pinger{
		stat:    &Stat{},
		network: UDP,
		closed:  make(chan interface{}),
	}
	err := p.SetAddr(host)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type packet struct {
	bytes  []byte
	nbytes int
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
	closed     chan interface{}
	OnRecv     func(*Packet)
	OnFinish   func(*Stat)
}

type Packet struct {
	Rtt    time.Duration
	IPAddr *net.IPAddr
	Nbytes int
	Seq    int
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
	handler := p.OnFinish
	if handler != nil {
		handler(stat)
	}
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

	var wg sync.WaitGroup
	wg.Add(1)

	proto := "ip4:icmp"
	if p.network == "udp" {
		proto = "udp4"
	}

	conn, err := icmp.ListenPacket(proto, p.sourceAddr)

	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}

	recv := make(chan *packet, 5)

	interval := time.NewTicker(p.Interval)

	go p.recvICMP(conn, recv, &wg)

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
			close(p.closed)
			return
		case <-ctxtimeout.Done():
			close(p.closed)
			return
		case <-interval.C:
			_ = p.sendICMP(conn)
		case r := <-recv:
			err := p.processPacket(r)
			if err != nil {
				fmt.Println("FATAL: ", err.Error())
			}
		case <-p.closed:
			wg.Wait()
			return
		}
	}
}
func (p *Pinger) recvICMP(conn *icmp.PacketConn, recv chan<- *packet, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-p.closed:
			return
		default:
			bytesGot := make([]byte, 512)
			conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
			n, _, err := conn.ReadFrom(bytesGot)
			if err != nil {
				if neterr, ok := err.(*net.OpError); ok {
					if neterr.Timeout() {
						continue
					} else {
						close(p.closed)
						return
					}
				}
			}
			recv <- &packet{bytes: bytesGot, nbytes: n}
		}
	}
}

func (p *Pinger) processPacket(packet *packet) error {
	bytesGot := packet.bytes
	n := packet.nbytes
	bytes := ipv4PayLoad(bytesGot)
	rm, err := icmp.ParseMessage(1, bytes[:n])
	if err != nil {
		return err
	}
	outpkt := &Packet{
		Nbytes: packet.nbytes,
		IPAddr: p.ipaddr,
	}
	var Rtt time.Duration
	switch pkt := rm.Body.(type) {
	case *icmp.Echo:
		Rtt = time.Since(bytesToTime(pkt.Data[:8]))
		outpkt.Rtt = Rtt
		Seq := pkt.Seq
		outpkt.Seq = Seq
		//fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n", n, p.ipaddr, Seq, Rtt)
	default:
		return fmt.Errorf("Error, invalid ICMP echo reply. Body type: %T, %s", pkt, pkt)
	}
	p.stat.PacketRecv += 1
	p.stat.Rtts = append(p.stat.Rtts, Rtt)
	if Rtt > p.stat.MaxRtt {
		p.stat.MaxRtt = Rtt
	}
	handler := p.OnRecv
	if handler != nil {
		handler(outpkt)
	}
	if p.stat.MinRtt == 0 || Rtt < p.stat.MinRtt {
		p.stat.MinRtt = Rtt
	}
	p.stat.SumRtt += Rtt
	p.stat.AvgRtt = p.stat.SumRtt / time.Duration(len(p.stat.Rtts))
	p.stat.SumSqrtRtt += (Rtt - p.stat.AvgRtt) * (Rtt - p.stat.AvgRtt)
	p.PacketRecv++
	if p.PacketRecv == p.Count {
		close(p.closed)
		return nil
	}
	return nil
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

func (p *Pinger) SetPrivileged(privileged bool) {
	if privileged {
		p.network = "ip"
	} else {
		p.network = "udp"
	}
}
