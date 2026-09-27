package hub

import "github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"

type MessageReceived struct {
	ChatID  string
	Message domain.Message
}
