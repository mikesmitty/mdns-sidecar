package mdns

import (
	"net"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/denisbrodbeck/machineid"
	"github.com/miekg/dns"
	nats "github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	ipv4mdns = "224.0.0.251"
	ipv6mdns = "ff02::fb"
	mdnsPort = 5353
	bufSize  = 65536
)

type Config struct {
	FilterTTL   int
	HighPort    bool
	ListenIP    string
	Monitor     []string
	PortFilters []string
	Queue       string
	UniqueID    string
}

type Server struct {
	config   Config
	uniqueID string

	ipv4CMs  []*ipv4.ControlMessage
	ipv4Dst  *net.UDPAddr
	ipv4High *ipv4.PacketConn
	ipv4Low  *ipv4.PacketConn

	ipv6Dst  *net.UDPAddr
	ipv6High *ipv6.PacketConn
	ipv6Low  *ipv6.PacketConn

	portRegex []*regexp.Regexp
	queue     *nats.EncodedConn
	wg        sync.WaitGroup
}

type Msg struct {
	Sender string
	Data   []byte
}

func StartServer(config Config) error {
	uniqueID, err := getUniqueID(config)
	if err != nil {
		return err
	}

	var portRegex []*regexp.Regexp
	for _, filter := range config.PortFilters {
		r, err := regexp.Compile(filter)
		if err != nil {
			log.Errorf("Error compiling port-filter regex '%s': %v", filter, err)
			continue
		}

		portRegex = append(portRegex, r)
	}

	ifs, err := getInterfaces(config)
	if err != nil {
		return err
	}

	cms, err := getCM4(config, ifs)
	if err != nil {
		return err
	}

	ipv4Low, err := listener4(config, ifs, mdnsPort)
	if err != nil {
		return err
	}
	ipv4High, err := listener4(config, ifs, 0)
	if err != nil {
		return err
	}

	ipv4Dst := &net.UDPAddr{
		IP:   net.ParseIP(ipv4mdns),
		Port: 5353,
	}

	s := &Server{
		config:    config,
		ipv4CMs:   cms,
		ipv4Dst:   ipv4Dst,
		ipv4High:  ipv4High,
		ipv4Low:   ipv4Low,
		portRegex: portRegex,
		uniqueID:  uniqueID,
	}

	c, err := getQueue(config)
	if err != nil {
		return err
	}
	s.queue = c

	_, err = s.queue.Subscribe("ipv4", s.send)
	if err != nil {
		return err
	}

	if ipv4Low != nil {
		s.wg.Add(1)
		go s.receive(ipv4Low)
	}

	if ipv4High != nil {
		s.wg.Add(1)
		go s.receive(ipv4High)
	}

	s.wg.Wait()

	return nil
}

// Start loop to pull multicast broadcasts off the wire and send them to NATS
func (s *Server) receive(p *ipv4.PacketConn) {
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

		if cm.TTL == s.config.FilterTTL {
			log.Debug("Discarding packet with filter TTL")
			log.Tracef("Discarding packet with filter TTL: %+v\n", cm)
			continue
		}

		msg := dns.Msg{}
		err = msg.Unpack(b[:n])
		if err != nil {
			log.Warnf("Error parsing packet from wire: %v", err)
		}
		log.Tracef("Received message from wire: %+v", msg)

		// TODO: Host blocklist checking / incoming filtering

		s.queue.Publish("ipv4", Msg{Sender: s.uniqueID, Data: b[:n]})
		log.Debug("Sent message to mesh")
	}
}

// Accept messages from NATS and send them out on the wire
func (s *Server) send(m *Msg) {
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

	// TODO: Add filtering logic

	var p *ipv4.PacketConn
	match := s.labelMatch(msg)
	if (s.config.HighPort && match) || (!s.config.HighPort && !match) {
		p = s.ipv4Low
		log.Debugf("Mesh message to low port, from sender: %s", m.Sender)
	} else {
		p = s.ipv4High
		log.Debugf("Mesh message to high port, from sender: %s", m.Sender)
	}

	for _, cm := range s.ipv4CMs {
		if _, err := p.WriteTo(m.Data, cm, s.ipv4Dst); err != nil {
			log.Errorf("Unable to send broadcast to wire: %v", err)
		}
	}

	log.Tracef("Rebroadcast message to wire: %+v", msg)
}

// Check if the DNS labels (question/answer name(s)) match our regex filters
func (s *Server) labelMatch(msg dns.Msg) bool {
	for _, r := range s.portRegex {
		for _, q := range msg.Question {
			name := strings.TrimSuffix(q.Name, ".")
			if r.MatchString(name) {
				return true
			}
		}
		for _, a := range msg.Answer {
			name := strings.TrimSuffix(a.Header().Name, ".")
			if r.MatchString(name) {
				return true
			}
		}
	}

	return false
}

// Connect to the NATS server with a JSON-encoded connection
func getQueue(config Config) (*nats.EncodedConn, error) {
	nc, err := nats.Connect(config.Queue)
	if err != nil {
		return nil, err
	}
	c, err := nats.NewEncodedConn(nc, nats.JSON_ENCODER)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// Get a unique ID so we don't repeat our own traffic
func getUniqueID(config Config) (string, error) {
	if config.UniqueID != "" {
		log.Warn("Using provided unique sender ID. If shared with other instances this could cause a self-DoS")
		return config.UniqueID, nil
	}

	uniqueID, err := machineid.ID()
	if uniqueID == "" || err != nil {
		log.Info("No machine id found, using hostname as sender id")

		uniqueID, err = os.Hostname()
		if uniqueID == "" || err != nil {
			log.Fatal("Unable to get machine id or hostname for use as sender id. Please provide a UniqueID")
			return "", err
		}
	}

	return uniqueID, nil
}
