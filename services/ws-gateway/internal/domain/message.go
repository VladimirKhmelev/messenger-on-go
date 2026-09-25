package domain

type Message struct {
	MessageID     string
	SenderUserID  string
	Text          string
	CreatedAtUnix int64
	EditedAtUnix  int64
	Deleted       bool
}
