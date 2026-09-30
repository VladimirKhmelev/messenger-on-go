package domain

// OutboxMessage is an event committed together with the change that caused
// it and not yet handed to the message broker.
type OutboxMessage struct {
	ID      int64
	Subject string
	Payload []byte
	Headers map[string][]string
}
