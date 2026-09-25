package domain

type PushSubscription struct {
	Endpoint  string
	P256dhKey string
	AuthKey   string
}

type PushPayload struct {
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
}
