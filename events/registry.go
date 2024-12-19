package events

type Handler interface {
	Handle(payload any) error
	Fail(err error)
}
type EventKey string

func (k EventKey) New(payload any) Event {
	return Event{
		Key:     k,
		Payload: payload,
	}
}

type Event struct {
	Key     EventKey
	Payload any
}

type Registry struct {
	handlers map[EventKey][]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[EventKey][]Handler),
	}
}

func (b *Registry) Register(key EventKey, handler Handler) {
	if _, ok := b.handlers[key]; !ok {
		b.handlers[key] = []Handler{}
	}

	b.handlers[key] = append(b.handlers[key], handler)
}

func (b *Registry) GetHandlers(key EventKey) []Handler {
	return b.handlers[key]
}
