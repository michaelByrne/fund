package events

import "github.com/nats-io/nats.go"

type NATSMessageBroker struct {
	nc *nats.Conn
}

func NewNATSMessageBroker(nc *nats.Conn) NATSMessageBroker {
	return NATSMessageBroker{nc: nc}
}

func (b *NATSMessageBroker) Publish(event string, data []byte) error {
	return b.nc.Publish(event, data)
}

func (b *NATSMessageBroker) Subscribe(event string, cb func(data []byte)) error {
	_, err := b.nc.Subscribe(event, func(msg *nats.Msg) {
		cb(msg.Data)
	})

	return err
}
