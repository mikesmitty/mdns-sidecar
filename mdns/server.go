package mdns

import (
	"net"
	"sync"

	nats "github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// TODO: Refactor this into something sensible

const (
	ipv4mdns = "224.0.0.251"
	ipv6mdns = "ff02::fb"
	mdnsPort = 5353
	bufSize  = 65536
	IPv4     = 4
	IPv6     = 6
)

type Config struct {
	Join     bool
	MagicTTL int
	Monitor  string
	Queue    string
	UniqueID string
}

type Server struct {
	config   Config
	uniqueID string

	queue    *nats.EncodedConn
	ipv4Addr net.IP
	ipv4List *ipv4.PacketConn
	ipv4Send *ipv4.PacketConn
	ipv6Addr net.IP
	ipv6List *ipv6.PacketConn
	ipv6Send *ipv6.PacketConn
	wg       sync.WaitGroup
}

type Msg struct {
	Sender string
	Data   []byte
}

// TODO: Find a use for this or nix it
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
