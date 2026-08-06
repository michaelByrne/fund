// Package messaging carries provider webhooks from the HTTP handler that receives
// them to the services that act on them.
//
// It used to be core NATS: in-process, no persistence, and Publish returning as
// soon as the message reached an outbound buffer. The webhook handler answered
// PayPal 200 on the strength of that, so a database error while recording a
// payment lost the payment permanently while the provider was told it had been
// delivered.
//
// It is now a JetStream stream on disk. Publish returns once the server has
// persisted the message, so the 200 means the event is safe rather than merely
// dispatched, and a handler that fails naks its message and is given it again.
//
// Not to be confused with service/fundevents, which is the audit trail of what
// happened to a fund. This is the delivery mechanism; that is the record.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// StreamName is the single stream every provider webhook lands in.
const StreamName = "WEBHOOKS"

// streamSubjects covers the provider event types in keys.go. PayPal names them
// with dots already, which is NATS's own subject separator, so they need no
// translation -- PAYMENT.SALE.COMPLETED is a subject as it stands.
var streamSubjects = []string{"PAYMENT.>", "BILLING.>"}

// retention keeps events for a week whether or not anything consumed them.
//
// Long enough to survive a weekend of the service being down, and to answer
// "what did PayPal actually send us" during an incident -- which the old bus
// could never do, because nothing was written down.
const retention = 7 * 24 * time.Hour

// publishTimeout bounds the wait for the server's persistence acknowledgement.
// The server is in-process, so this guards against a wedged stream holding the
// webhook handler open rather than describing a normal latency budget.
const publishTimeout = 10 * time.Second

// ackWait is how long a handler has before its message is treated as undelivered
// and handed out again. Handlers do database work and nothing more.
const ackWait = 30 * time.Second

// defaultBackOff is the delay before each redelivery. MaxDeliver is one more
// than its length, so a message is tried once and then retried on each of these
// delays before JetStream stops offering it.
var defaultBackOff = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

// maxRecordedExhausted bounds the in-memory record of messages JetStream gave up
// on. Fifty is far more than a healthy system produces and small enough to keep.
const maxRecordedExhausted = 50

// Exhausted is a message JetStream stopped redelivering because it used up
// MaxDeliver attempts. It is the only silent loss the durable bus still has, so
// it is worth surfacing somewhere a person will look.
type Exhausted struct {
	Consumer   string
	StreamSeq  uint64
	Deliveries int
	At         time.Time
}

// Status is what the admin page renders.
type Status struct {
	Stream    StreamStatus
	Consumers []ConsumerStatus
	// Exhausted is since this process started, because it is held in memory. A
	// restart clears it; the messages themselves are still in the stream until
	// retention expires them.
	Exhausted []Exhausted
}

type StreamStatus struct {
	Name     string
	Messages uint64
	Bytes    uint64
	FirstSeq uint64
	LastSeq  uint64
	Oldest   time.Time
	Newest   time.Time
}

type ConsumerStatus struct {
	Name    string
	Subject string
	// Pending is waiting to be delivered. AckPending has been delivered and is
	// being worked on, or was delivered to something that died. Redelivered is
	// the count that have been handed out more than once, which is the number
	// worth watching: it is zero on a healthy consumer.
	Pending     uint64
	AckPending  int
	Redelivered int
	AckFloor    uint64
	Delivered   uint64
}

type Broker struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	stream jetstream.Stream

	ctx    context.Context
	logger *slog.Logger

	// backOff is a field rather than a constant so tests can shorten it. Its
	// values are the difference between retrying a failing database and
	// hammering one.
	backOff []time.Duration

	consuming []jetstream.ConsumeContext
	// advisories is kept so Close can stop it. A subscription left running can
	// fire its callback after the broker is closed, appending to state nothing
	// owns any more.
	advisories *nats.Subscription

	// names maps durable back to the subject it was created for, because
	// ConsumerInfo reports the durable name and an operator thinks in event types.
	mu        sync.Mutex
	names     map[string]string
	exhausted []Exhausted
}

// NewBroker declares the stream. CreateOrUpdateStream is idempotent, so a restart
// reattaches to the existing stream and its unacknowledged messages rather than
// starting empty.
func NewBroker(ctx context.Context, nc *nats.Conn, logger *slog.Logger) (*Broker, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to open jetstream: %w", err)
	}

	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      StreamName,
		Subjects:  streamSubjects,
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		MaxAge:    retention,
		Discard:   jetstream.DiscardOld,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to declare stream %s: %w", StreamName, err)
	}

	broker := &Broker{
		nc:      nc,
		js:      js,
		stream:  stream,
		ctx:     ctx,
		logger:  logger,
		backOff: defaultBackOff,
		names:   map[string]string{},
	}

	if err = broker.watchExhausted(); err != nil {
		return nil, err
	}

	return broker, nil
}

// watchExhausted records the messages JetStream stops redelivering.
//
// Every other failure in this package is recoverable: a nak comes back, a
// restart resumes. Running out of deliveries is where an event finally stops,
// and without this it is a single log line at the moment it happens.
func (b *Broker) watchExhausted() error {
	subject := fmt.Sprintf("$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.%s.*", StreamName)

	sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
		var advisory struct {
			Consumer   string `json:"consumer"`
			StreamSeq  uint64 `json:"stream_seq"`
			Deliveries int    `json:"deliveries"`
		}

		if errParse := json.Unmarshal(msg.Data, &advisory); errParse != nil {
			b.logger.Error("failed to read max-deliveries advisory",
				slog.String("error", errParse.Error()))

			return
		}

		b.logger.Error("giving up on a message after exhausting redelivery",
			slog.String("consumer", advisory.Consumer),
			slog.Uint64("stream_seq", advisory.StreamSeq),
			slog.Int("deliveries", advisory.Deliveries),
		)

		b.mu.Lock()
		defer b.mu.Unlock()

		b.exhausted = append(b.exhausted, Exhausted{
			Consumer:   advisory.Consumer,
			StreamSeq:  advisory.StreamSeq,
			Deliveries: advisory.Deliveries,
			At:         time.Now(),
		})

		if len(b.exhausted) > maxRecordedExhausted {
			b.exhausted = b.exhausted[len(b.exhausted)-maxRecordedExhausted:]
		}
	})
	if err != nil {
		return fmt.Errorf("failed to watch for exhausted messages: %w", err)
	}

	b.advisories = sub

	return nil
}

// Status reports what the stream holds and how far each consumer has got.
func (b *Broker) Status(ctx context.Context) (Status, error) {
	info, err := b.stream.Info(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("failed to read stream info: %w", err)
	}

	status := Status{
		Stream: StreamStatus{
			Name:     info.Config.Name,
			Messages: info.State.Msgs,
			Bytes:    info.State.Bytes,
			FirstSeq: info.State.FirstSeq,
			LastSeq:  info.State.LastSeq,
			Oldest:   info.State.FirstTime,
			Newest:   info.State.LastTime,
		},
	}

	consumers := b.stream.ListConsumers(ctx)
	for consumer := range consumers.Info() {
		status.Consumers = append(status.Consumers, ConsumerStatus{
			Name:        consumer.Name,
			Subject:     b.subjectFor(consumer.Name, consumer.Config.FilterSubject),
			Pending:     consumer.NumPending,
			AckPending:  consumer.NumAckPending,
			Redelivered: consumer.NumRedelivered,
			AckFloor:    consumer.AckFloor.Stream,
			Delivered:   consumer.Delivered.Stream,
		})
	}

	if err = consumers.Err(); err != nil {
		return Status{}, fmt.Errorf("failed to list consumers: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	status.Exhausted = append(status.Exhausted, b.exhausted...)

	return status, nil
}

// subjectFor prefers the subject we registered, falling back to whatever the
// consumer reports. A durable created by an older build may have no filter.
func (b *Broker) subjectFor(durable, filter string) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subject, ok := b.names[durable]; ok {
		return subject
	}

	return filter
}

// Publish returns only once the message is on disk.
//
// This is the difference the whole change rests on. The webhook handler answers
// PayPal 200 immediately afterwards, and that answer is a promise the event will
// be handled; core NATS returned before anything was persisted, so the promise
// was empty.
func (b *Broker) Publish(event string, data []byte) error {
	ctx, cancel := context.WithTimeout(b.ctx, publishTimeout)
	defer cancel()

	if _, err := b.js.Publish(ctx, event, data); err != nil {
		return fmt.Errorf("failed to publish %s: %w", event, err)
	}

	return nil
}

// Subscribe attaches a durable consumer to one event type.
//
// Durable, so a consumer that restarts resumes where it left off rather than
// re-reading the stream or skipping what arrived while it was gone. A handler
// returning an error naks the message and JetStream redelivers it on the backoff
// schedule; returning nil acknowledges it and it is not seen again.
func (b *Broker) Subscribe(event string, cb func(data []byte) error) error {
	consumer, err := b.stream.CreateOrUpdateConsumer(b.ctx, jetstream.ConsumerConfig{
		Durable:       durableName(event),
		FilterSubject: event,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxDeliver:    len(b.backOff) + 1,
		BackOff:       b.backOff,
	})
	if err != nil {
		return fmt.Errorf("failed to create consumer for %s: %w", event, err)
	}

	consuming, err := consumer.Consume(func(msg jetstream.Msg) {
		if errHandle := cb(msg.Data()); errHandle != nil {
			b.logger.Error("handler failed, message will be redelivered",
				slog.String("event", event),
				slog.String("error", errHandle.Error()),
			)

			// NakWithDelay, not Nak. A plain nak asks for the message back
			// immediately, so a handler failing because the database is down would
			// burn its whole delivery budget in milliseconds and park the message
			// -- retrying hardest at the moment the system can least afford it.
			// ConsumerConfig.BackOff only governs redelivery after an ack timeout,
			// which is not the path an explicit nak takes.
			if errNak := msg.NakWithDelay(b.nakDelay(msg)); errNak != nil {
				b.logger.Error("failed to nak message",
					slog.String("event", event),
					slog.String("error", errNak.Error()),
				)
			}

			return
		}

		if errAck := msg.Ack(); errAck != nil {
			// The work is done but the acknowledgement did not land, so the message
			// arrives again. Handlers are idempotent, which is what makes that
			// survivable rather than a second payment.
			b.logger.Error("failed to ack a handled message",
				slog.String("event", event),
				slog.String("error", errAck.Error()),
			)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to consume %s: %w", event, err)
	}

	b.consuming = append(b.consuming, consuming)

	b.mu.Lock()
	b.names[durableName(event)] = event
	b.mu.Unlock()

	return nil
}

// nakDelay is how long to wait before this message is offered again, taken from
// how many times it has already been delivered. Falls back to the longest delay
// when the metadata cannot be read, since guessing short is what causes the tight
// loop this exists to avoid.
func (b *Broker) nakDelay(msg jetstream.Msg) time.Duration {
	longest := b.backOff[len(b.backOff)-1]

	meta, err := msg.Metadata()
	if err != nil {
		return longest
	}

	attempt := int(meta.NumDelivered) - 1
	if attempt < 0 || attempt >= len(b.backOff) {
		return longest
	}

	return b.backOff[attempt]
}

// Close stops consuming. Messages in flight are neither acked nor naked, so they
// are redelivered after AckWait rather than lost.
func (b *Broker) Close() {
	for _, consuming := range b.consuming {
		consuming.Stop()
	}

	if b.advisories != nil {
		if err := b.advisories.Unsubscribe(); err != nil {
			b.logger.Error("failed to unsubscribe from advisories", slog.String("error", err.Error()))
		}
	}
}

// durableName turns a subject into a name JetStream will accept. Durable names
// may not contain dots, spaces or the wildcard characters, and every subject here
// has dots in it.
func durableName(event string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, event)
}
