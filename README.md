Here is a very simple example that sends & receive 3 packets:
```go

pinger, err := ping.NewPinger("www.google.com")
if err != nil {
    fmt.Printf("ERROR: %s\n", err.Error())
	return
}

pinger.count = 3
pinger.Run()
```

Here is an example that emulates that unix ping command:
```go
pinger, err := NewPinger(host)
if err != nil {
    fmt.Printf("ERROR: %s\n", err.Error())
    return
}
pinger.OnRecv = func(pkt *Packet) {
    fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v\n", pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
}

pinger.OnFinish = func(stat *Stat) {
    fmt.Printf("\n--- %s ping statistics ---\n", pinger.addr)
    fmt.Printf("%d packets transmitted, %d packets received, %v%% packet loss\n", stat.PacketSent, stat.PacketRecv, stat.PacketLoss)
    fmt.Printf("round-trip min/avg/max/stddev = %v/%v/%v/%v\n", stat.MinRtt, stat.AvgRtt, stat.MaxRtt, stat.StdDevRtt)
}
fmt.Printf("PING %s (%s)\n", pinger.Addr(), pinger.IPAddr())
pinger.Run()
```
It sends ICMP packet(s) and waits for a response. If it receives a response, it calls the "receive" callback. When it's finished, it calls the "finish" callback.

For a full ping example, see "cmd/ping/ping.go".