package mdns

import (
	"context"
	"net"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
)

// TODO: Add ipv6 support
// TODO: Refactor this into something sensible
// Inspired by github.com/hashicorp/mdns
func sender4(config Config) (*ipv4.PacketConn, error) {
	netConf := &net.ListenConfig{Control: reusePort}
	c, err := netConf.ListenPacket(context.Background(), "udp4", config.SenderIP)
	if err != nil {
		return nil, err
	}

	p := ipv4.NewPacketConn(c)
	if config.Join {
		group := net.ParseIP(ipv4mdns)

		var ifi *net.Interface
		if config.Monitor != "" {
			ifi, err = net.InterfaceByName(config.Monitor)
			if err != nil {
				return nil, err
			}
		}

		if err := p.JoinGroup(ifi, &net.UDPAddr{IP: group}); err != nil {
			c.Close()
			return nil, err
		}

		// Try to disable multicast loopback so we don't have to filter our own traffic
		p.SetMulticastLoopback(false)
	}

	// Enable the ControlMessage struct so we can get the source IP and IP TTL
	if err := p.SetControlMessage(ipv4.FlagTTL|ipv4.FlagSrc|ipv4.FlagDst, true); err != nil {
		c.Close()
		return nil, err
	}

	log.Infof("Sending on interface: %s", config.Monitor)
	log.Debugf("Sending on interface %s with address: %s", config.Monitor, p.LocalAddr())

	return p, nil
}

func (s *Server) send(p *ipv4.PacketConn) error {
	group := net.ParseIP(ipv4mdns)
	dst := &net.UDPAddr{IP: group, Port: 5353}
	err := p.SetMulticastTTL(s.config.MagicTTL)
	if err != nil {
		log.Error(err)
	}

	_, err = s.queue.Subscribe("ipv4", func(m *Msg) {
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
