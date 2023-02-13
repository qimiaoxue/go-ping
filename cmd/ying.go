package main

import (
	"flag"
	"fmt"
	"time"

	ping "github.com/qimiaoxue/ying"
)

var usage = `
Usage:
	ping [-c count] [-i interval] [-t timeout] [--privileged] host
	
Example:
# ping google continuely
ping www.google.com

# ping google 5 times
ping -c 5 www.google.com

#ping google 5 times at 500ms intervals
ping -c 5 -i 500ms www.google.com

$ping google for 10 seconds
ping -t 10s www.google.com

#ping a google raw ICMP ping
sudo ping --privileged www.google.com
`

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
	pinger, err := ping.NewPinger(host)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
		return
	}

	pinger.OnRecv = func(pkt *ping.Packet) {
		fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n", pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
	}

	pinger.OnFinish = func(stat *ping.Stat) {
		fmt.Printf("\n--- %s ping statistics ---\n", pinger.Addr())
		fmt.Printf("%d packets transmitted, %d packets received, %v%% packet loss\n", stat.PacketSent, stat.PacketRecv, stat.PacketLoss)
		fmt.Printf("round-trip min/avg/max/stddev = %v/%v/%v/%v\n", stat.MinRtt, stat.AvgRtt, stat.MaxRtt, stat.StdDevRtt)

	}
	fmt.Printf("PING %s (%s)\n", pinger.Addr(), pinger.IPAddr())
	pinger.Count = *count
	pinger.Interval = *interval
	pinger.Timeout = *timeout
	pinger.SetPrivileged(*privileged)
	pinger.Run()
}
