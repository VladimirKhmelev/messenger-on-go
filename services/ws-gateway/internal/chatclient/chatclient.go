package chatclient

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
)

type Message struct {
	MessageID     string
	SenderUserID  string
	Text          string
	CreatedAtUnix int64
	EditedAtUnix  int64
	Deleted       bool
}

type Client struct {
	conn           *grpc.ClientConn
	chat           chatv1.ChatServiceClient
	internalSecret string
}

func Dial(addr, internalSecret string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, chat: chatv1.NewChatServiceClient(conn), internalSecret: internalSecret}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) SendMessage(ctx context.Context, bearerToken, chatID, text string) (string, error) {
	ctx = withBearerToken(ctx, bearerToken)

	resp, err := c.chat.SendMessage(ctx, &chatv1.SendMessageRequest{ChatId: chatID, Text: text})
	if err != nil {
		return "", err
	}
	return resp.GetMessageId(), nil
}

func (c *Client) GetHistory(ctx context.Context, bearerToken, chatID string, limit int32) ([]Message, error) {
	ctx = withBearerToken(ctx, bearerToken)

	resp, err := c.chat.GetHistory(ctx, &chatv1.GetHistoryRequest{ChatId: chatID, Limit: limit})
	if err != nil {
		return nil, err
	}

	messages := make([]Message, 0, len(resp.GetMessages()))
	for _, m := range resp.GetMessages() {
		messages = append(messages, toClientMessage(m))
	}
	return messages, nil
}

func (c *Client) ListMembers(ctx context.Context, chatID string) ([]string, error) {
	ctx = c.withInternalSecret(ctx)

	resp, err := c.chat.ListMembers(ctx, &chatv1.ListMembersRequest{ChatId: chatID})
	if err != nil {
		return nil, err
	}
	return resp.GetUserIds(), nil
}

func (c *Client) GetPresence(ctx context.Context, userID string) (online bool, lastSeenUnix int64, err error) {
	ctx = c.withInternalSecret(ctx)

	resp, err := c.chat.GetPresence(ctx, &chatv1.GetPresenceRequest{UserId: userID})
	if err != nil {
		return false, 0, err
	}
	return resp.GetOnline(), resp.GetLastSeenUnix(), nil
}

func (c *Client) SetOffline(ctx context.Context, userID string) error {
	ctx = c.withInternalSecret(ctx)

	_, err := c.chat.SetOffline(ctx, &chatv1.SetOfflineRequest{UserId: userID})
	return err
}

func (c *Client) ListContacts(ctx context.Context, userID string) ([]string, error) {
	ctx = c.withInternalSecret(ctx)

	resp, err := c.chat.ListContacts(ctx, &chatv1.ListContactsRequest{UserId: userID})
	if err != nil {
		return nil, err
	}
	return resp.GetUserIds(), nil
}

func (c *Client) GetMessage(ctx context.Context, messageID string) (Message, error) {
	ctx = c.withInternalSecret(ctx)

	resp, err := c.chat.GetMessage(ctx, &chatv1.GetMessageRequest{MessageId: messageID})
	if err != nil {
		return Message{}, err
	}
	return toClientMessage(resp.GetMessage()), nil
}

func (c *Client) EditMessage(ctx context.Context, bearerToken, chatID, messageID, text string) error {
	ctx = withBearerToken(ctx, bearerToken)

	_, err := c.chat.EditMessage(ctx, &chatv1.EditMessageRequest{ChatId: chatID, MessageId: messageID, Text: text})
	return err
}

func (c *Client) DeleteMessageForAll(ctx context.Context, bearerToken, chatID, messageID string) error {
	ctx = withBearerToken(ctx, bearerToken)

	_, err := c.chat.DeleteMessageForAll(ctx, &chatv1.DeleteMessageForAllRequest{ChatId: chatID, MessageId: messageID})
	return err
}

func (c *Client) DeleteMessageForMe(ctx context.Context, bearerToken, chatID, messageID string) error {
	ctx = withBearerToken(ctx, bearerToken)

	_, err := c.chat.DeleteMessageForMe(ctx, &chatv1.DeleteMessageForMeRequest{ChatId: chatID, MessageId: messageID})
	return err
}

func toClientMessage(m *chatv1.Message) Message {
	return Message{
		MessageID:     m.GetMessageId(),
		SenderUserID:  m.GetSenderUserId(),
		Text:          m.GetText(),
		CreatedAtUnix: m.GetCreatedAtUnix(),
		EditedAtUnix:  m.GetEditedAtUnix(),
		Deleted:       m.GetDeleted(),
	}
}

func (c *Client) withInternalSecret(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-internal-secret", c.internalSecret)
}

func withBearerToken(ctx context.Context, bearerToken string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+bearerToken)
}
