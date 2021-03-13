package mdns

import (
	"context"
	"fmt"
	"net"
	"syscall"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

// TODO: Add ipv6 support

func listener4(config Config, port int) (*ipv4.PacketConn, error) {
	p, err := getConn(config, port)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			p.Close()
		}
	}()

	ifs, err := getInterfaces(config)
	if err != nil {
		return nil, err
	}

	err = joinMulticast(p, ifs)
	if err != nil {
		return nil, err
	}

	err = p.SetMulticastTTL(config.FilterTTL)
	if err != nil {
		log.Error(err)
	}

	// Enable the ControlMessage struct so we can get the source IP and IP TTL
	if err := p.SetControlMessage(ipv4.FlagTTL|ipv4.FlagSrc|ipv4.FlagDst, true); err != nil {
		return nil, err
	}

	log.Infof("Listening on interface %s with address: %s", config.Monitor, p.LocalAddr())

	return p, nil
}

func getConn(config Config, port int) (*ipv4.PacketConn, error) {
	listenAddr := fmt.Sprintf("%s:%d", config.ListenIP, port)

	netConf := &net.ListenConfig{Control: reusePort}
	c, err := netConf.ListenPacket(context.Background(), "udp4", listenAddr)
	if err != nil {
		return nil, err
	}

	p := ipv4.NewPacketConn(c)

	return p, nil
}

func getInterfaces(config Config) (ifs []*net.Interface, err error) {
	for _, ifn := range config.Monitor {
		ifi, err := net.InterfaceByName(ifn)
		if err != nil {
			return nil, err
		}

		ifs = append(ifs, ifi)
	}

	if len(ifs) == 0 {
		ifaces, err := net.Interfaces()
		if err != nil {
			return nil, err
		}

		for i := range ifaces {
			ifs = append(ifs, &ifaces[i])
		}
	}

	return ifs, nil
}

func joinMulticast(p *ipv4.PacketConn, ifs []*net.Interface) error {
	group := net.ParseIP(ipv4mdns)

	for i := range ifs {
		if err := p.JoinGroup(ifs[i], &net.UDPAddr{IP: group}); err != nil {
			return err
		}
	}

	// Try to disable multicast loopback so we don't have to filter our own traffic
	err := p.SetMulticastLoopback(false)
	if err != nil {
		log.Warnf("error disabling multicast loopback: %v", err)
	}

	return nil
}

func reusePort(network, address string, conn syscall.RawConn) error {
	return conn.Control(func(descriptor uintptr) {
		syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
}
