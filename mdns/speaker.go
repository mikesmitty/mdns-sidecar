package mdns

import (
	"net"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
)

// TODO: Add ipv6 support
// TODO: Refactor this into something sensible
// Inspired by github.com/hashicorp/mdns

func (s *Server) send(p *ipv4.PacketConn) error {
	group := net.ParseIP(ipv4mdns)
	dst := &net.UDPAddr{IP: group, Port: 5353}
	p.SetMulticastTTL(s.config.MagicTTL)

	_, err := s.queue.Subscribe("ipv4", func(m *Msg) {
		if m.Sender == s.uniqueID {
			log.Debug("Ignoring mesh message from self")
			return
		}

		msg := dns.Msg{}
		err := msg.Unpack(m.Data)
		if err != nil {
			log.Warnf("Error parsing mesh packet: %v", err)
			return
		}

		log.Debugf("Mesh message from sender: %s", m.Sender)
		log.Tracef("Rebroadcast message to wire: %+v", msg)

		if _, err := p.WriteTo(m.Data, nil, dst); err != nil {
			log.Errorf("Unable to send broadcast to wire: %v", err)
		}
	})
	return err
}
