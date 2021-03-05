package mdns

import nats "github.com/nats-io/nats.go"

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
