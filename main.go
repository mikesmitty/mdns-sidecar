package main

import (
	"time"

	"github.com/mikesmitty/mdns-sidecar/mdns"
	log "github.com/sirupsen/logrus"
)

const bufSize = 65536

func main() {
	//m := new(dns.Msg)
	//m.SetQuestion("s31_02.local.", dns.TypeA)
	//m.RecursionDesired = false

	//c := new(dns.Client)
	//in, rtt, err := c.Exchange(m, "224.0.0.251:5353")
	//fmt.Printf("%v | %v | %v\n", in, rtt, err)

	log.SetLevel(log.DebugLevel)

	mdns.NewListener(mdns.Config{Monitor: "eth0"})

	time.Sleep(5 * time.Minute)
}
