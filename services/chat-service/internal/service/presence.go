package service

import "context"

func (s *ChatService) GetPresence(ctx context.Context, userID string) (online bool, lastSeenUnix int64, err error) {
	online, err = s.presence.IsOnline(ctx, userID)
	if err != nil {
		return false, 0, err
	}

	lastSeenUnix, err = s.presence.LastSeen(ctx, userID)
	if err != nil {
		return false, 0, err
	}

	return online, lastSeenUnix, nil
}

func (s *ChatService) SetOnline(ctx context.Context, userID string) error {
	return s.presence.SetOnline(ctx, userID)
}

func (s *ChatService) SetOffline(ctx context.Context, userID string) error {
	return s.presence.SetOffline(ctx, userID)
}

func (s *ChatService) SetTyping(ctx context.Context, chatID, userID string) error {
	return s.presence.SetTyping(ctx, chatID, userID)
}

func (s *ChatService) GetTyping(ctx context.Context, chatID, userID string) (bool, error) {
	return s.presence.IsTyping(ctx, chatID, userID)
}
