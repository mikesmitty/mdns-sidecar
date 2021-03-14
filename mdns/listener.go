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

// Start an IPv4 listener to send/receive broadcasts
func listener4(config Config, ifs []*net.Interface, port int) (*ipv4.PacketConn, error) {
	p, err := getConn(config, port)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			p.Close()
		}
	}()

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

// Create a PacketConn that can reuse sockets where necessary (e.g. broadcaster is on localhost)
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

// Generate net.ControlMessage structs for each interface to dictate where broadcasts are sent
func getCM4(config Config, ifs []*net.Interface) ([]*ipv4.ControlMessage, error) {
	var cms []*ipv4.ControlMessage

	ip := net.IPv4zero
	if config.ListenIP != "" {
		ip = net.ParseIP(config.ListenIP)
		if ip == nil {
			return nil, fmt.Errorf("Couldn't parse listen-ip: %s", config.ListenIP)
		}
	}
	for i := range ifs {
		cms = append(cms, &ipv4.ControlMessage{Src: ip, IfIndex: ifs[i].Index})
	}

	return cms, nil
}

// Get net.Interfaces for the list of provided interface names or all interfaces
func getInterfaces(config Config) (ifs []*net.Interface, err error) {
	for _, ifn := range config.Monitor {
		ifi, err := net.InterfaceByName(ifn)
		if err != nil {
			return nil, err
		}

		log.Debugf("Adding %s to monitored interfaces", ifi.Name)
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

// Send out multicast group join messages on the wire for a list of interfaces
func joinMulticast(p *ipv4.PacketConn, ifs []*net.Interface) error {
	group := net.ParseIP(ipv4mdns)

	for i := range ifs {
		if err := p.JoinGroup(ifs[i], &net.UDPAddr{IP: group}); err != nil {
			return err
		}
		log.Debugf("Joining multicast on %s", ifs[i].Name)
	}

	// Try to disable multicast loopback so we don't have to filter our own traffic
	err := p.SetMulticastLoopback(false)
	if err != nil {
		log.Warnf("error disabling multicast loopback: %v", err)
	}

	return nil
}

// Allow reuse of a port for when necessary in getConn()
func reusePort(network, address string, conn syscall.RawConn) error {
	return conn.Control(func(descriptor uintptr) {
		syscall.SetsockoptInt(int(descriptor), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
}
