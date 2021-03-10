package mdns

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/denisbrodbeck/machineid"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

// TODO: Add ipv6 support
// TODO: Refactor this into something sensible
// Inspired by github.com/hashicorp/mdns

func StartServer(config Config) error {
	ipv4List, err := listener4(config)
	//ipv6List, _ := listener6(config)

	//if ipv4List == nil && ipv6List == nil {
	if ipv4List == nil {
		return fmt.Errorf("No multicast listeners could be started: %w", err)
	}

	ipv4Send, err := sender4(config)
	//ipv6Send, _ := sender6(config)

	//if ipv4Send == nil && ipv6Send == nil {
	if ipv4Send == nil {
		return fmt.Errorf("No multicast senders could be started: %w", err)
	}

	// TODO: Function this section out
	var uniqueID string
	//var err error
	if config.UniqueID != "" {
		log.Warn("Using provided unique sender ID. If shared with other instances this could cause a self-DoS")
	} else {
		uniqueID, err = machineid.ID()
		if uniqueID == "" || err != nil {
			log.Info("No machine id found, using hostname as sender id")

			uniqueID, err = os.Hostname()
			if uniqueID == "" || err != nil {
				log.Fatal("Unable to get machine id or hostname for use as sender id. Please provide a UniqueID")
				return err
			}
		}
	}

	s := &Server{
		config:   config,
		ipv4List: ipv4List,
		//ipv6List: ipv6List,
		ipv4Send: ipv4Send,
		//ipv6Send: ipv6Send,
		uniqueID: uniqueID,
	}

	c, err := getQueue(config)
	if err != nil {
		return err
	}
	s.queue = c

	if s.config.HighPort {
		err = s.send(ipv4Send)
	} else {
		err = s.send(ipv4List)
	}
	if err != nil {
		return err
	}

	if ipv4List != nil {
		s.wg.Add(1)
		go s.recv(ipv4List)
	}

	// TODO ipv6
	//if ipv6List != nil {
	//  s.wg.Add(1)
	//	go s.recv(ipv6List)
	//}

	s.wg.Wait()

	return nil
}

// TODO: Clean up NIC handling, allow multiple NICs, etc.
func listener4(config Config) (*ipv4.PacketConn, error) {
	netConf := &net.ListenConfig{Control: reusePort}
	c, err := netConf.ListenPacket(context.Background(), "udp4", ":5353")
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

	log.Infof("Listening on interface: %s", config.Monitor)
	log.Debugf("Listening on interface %s with address: %s", config.Monitor, p.LocalAddr())

	return p, nil
}

func (s *Server) recv(p *ipv4.PacketConn) {
	defer s.wg.Done()

	for {
		b := make([]byte, bufSize)
		n, cm, _, err := p.ReadFrom(b)
		if err != nil {
			log.Errorf("Error reading packet from wire: %v", err)
			continue
		}

		if cm == nil {
			log.Error("Received no ControlMessage from packet")
			continue
		}

		if cm.TTL == s.config.MagicTTL {
			log.Debug("Discarding packet with magic TTL")
			log.Tracef("Discarding packet with magic TTL: %+v\n", cm)
			continue
		}

		msg := dns.Msg{}
		err = msg.Unpack(b[:n])
		if err != nil {
			log.Warnf("Error parsing packet from wire: %v", err)
		}
		log.Tracef("Received message from wire: %+v", msg)

		// TODO: Host blocklist checking

		s.queue.Publish("ipv4", Msg{Sender: s.uniqueID, Data: b[:n]})
		log.Debug("Sent message to mesh")
	}
}

func reusePort(network, address string, conn syscall.RawConn) error {
	return conn.Control(func(descriptor uintptr) {
		syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
}
