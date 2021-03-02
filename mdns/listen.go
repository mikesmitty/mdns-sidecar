package mdns

import (
	"fmt"
	"net"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
)

// OLD SETUP, DELETE LATER

//const bufSize = 65536

func Listen(iface string) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		log.Fatal("Couldn't get interface %s: %v", iface, err)
	}

	addr := "0.0.0.0:5353"
	group := net.IPv4(224, 0, 0, 251)

	c, err := net.ListenPacket("udp4", addr)
	if err != nil {
		log.Fatal("Couldn't start listener on %s: %v", addr, err)
	}
	defer c.Close()

	p := ipv4.NewPacketConn(c)
	if err := p.JoinGroup(ifi, &net.UDPAddr{IP: group}); err != nil {
		log.Fatal("Couldn't join multicast group on %s: %v", group, err)
	}

	if err := p.SetControlMessage(ipv4.FlagTTL|ipv4.FlagSrc|ipv4.FlagDst, true); err != nil {
		log.Fatal("Failed to enable flags in SetControlMessage: %v", err)
	}

	for {
		b := make([]byte, bufSize)
		n, cm, _, err := p.ReadFrom(b)
		if err != nil {
			log.Debugf("Error reading packet: %v", err)
			continue
		}

		// TODO: Check incoming packet TTL / avoid packet storm generation
		if cm != nil {
			fmt.Printf("ttl: %d src: %s dst: %s\n", cm.TTL, cm.Src, cm.Dst)
		} else {
			log.Debugf("Received no ControlMessage from packet")
		}

		msg := dns.Msg{}
		err = msg.Unpack(b[:n])
		if err != nil {
			log.Debugf("Error parsing packet: %v", err)
			continue
		}

		fmt.Println(msg.String())
	}
}
