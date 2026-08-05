// Package messaging carries provider webhooks from the HTTP handler that receives
// them to the services that act on them. It is transport, not a record.
//
// Not to be confused with service/fundevents, which is the audit trail of what
// happened to a fund. This bus is in-process and has no persistence, so anything
// published here can be lost on restart; anything that must survive gets written
// to the database instead.
package messaging

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
