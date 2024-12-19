package events

type EventHandler struct {
	registry *Registry
}

func NewEventHandler(registry *Registry) *EventHandler {
	return &EventHandler{
		registry: registry,
	}
}

func (h EventHandler) Handle(event Event) {
	for _, handler := range h.registry.GetHandlers(event.Key) {
		go func() {
			err := handler.Handle(event.Payload)
			if err != nil {
				handler.Fail(err)
			}
		}()
	}
}
