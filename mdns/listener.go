package mdns

import (
	"fmt"
	"net"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// TODO: Add ipv6 support
// TODO: Refactor this into something sensible
// Inspired by github.com/hashicorp/mdns

const (
	ipv4mdns = "224.0.0.251"
	ipv6mdns = "ff02::fb"
	mdnsPort = 5353
	bufSize  = 65536
	IPv4     = 4
	IPv6     = 6
)

type Config struct {
	Monitor string
	Listen  string

	// Defaulting to zero for now
	MagicTTL int
}

type Server struct {
	config Config

	ipv4List *ipv4.PacketConn
	ipv6List *ipv6.PacketConn
}

func NewServer(config Config) (*Server, error) {
	ipv4List, _ := listener4(config)
	ipv6List, _ := listener6(config)

	if ipv4List == nil && ipv6List == nil {
		return nil, fmt.Errorf("No multicast listeners could be started")
	}

	s := &Server{
		config:   config,
		ipv4List: ipv4List,
		ipv6List: ipv6List,
	}

	if ipv4List != nil {
		go s.recv(ipv4List)
	}

	// TODO ipv6
	//if ipv6List != nil {
	//	go s.recv(ipv6List)
	//}

	return s, nil
}

func listener4(config Config) (*ipv4.PacketConn, error) {
	group := net.ParseIP(ipv4mdns)

	ifi, err := net.InterfaceByName(config.Monitor)
	if err != nil {
		return nil, err
	}

	c, err := net.ListenPacket("udp4", ":5353")
	if err != nil {
		return nil, err
	}

	p := ipv4.NewPacketConn(c)
	if err := p.JoinGroup(ifi, &net.UDPAddr{IP: group}); err != nil {
		c.Close()
		return nil, err
	}

	if err := p.SetControlMessage(ipv4.FlagTTL|ipv4.FlagSrc|ipv4.FlagDst, true); err != nil {
		c.Close()
		return nil, err
	}

	return p, nil
}

// TODO ipv6
func listener6(config Config) (*ipv6.PacketConn, error) {
	return nil, nil
}

func (s *Server) recv(p *ipv4.PacketConn) {
	for {
		b := make([]byte, bufSize)
		n, cm, _, err := p.ReadFrom(b)
		if err != nil {
			log.Debugf("Error reading packet: %v", err)
			continue
		}

		if cm == nil {
			log.Debugf("Received no ControlMessage from packet")
			continue
		}

		if cm.TTL == s.Config.MagicTTL {
			log.Debugf("Discarding packet with magic TTL")
			continue
		}

		// TODO: Host blocklist checking

		msg := dns.Msg{}
		err = msg.Unpack(b[:n])
		if err != nil {
			log.Debugf("Error parsing packet: %v", err)
			continue
		}

		fmt.Println(msg.String())
	}
}

func isLocal(addr net.Addr) bool {
	localAddrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal("Couldn't get interface addresses: %v", err)
	}

	for i := range localAddrs {
		if addr == localAddrs[i] {
			log.Debugf("Ignoring local address: %s", addr)
			return true
		}
	}

	return false
}
