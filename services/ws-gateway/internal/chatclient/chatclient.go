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
}

type Client struct {
	conn *grpc.ClientConn
	chat chatv1.ChatServiceClient
}

func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, chat: chatv1.NewChatServiceClient(conn)}, nil
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
		messages = append(messages, Message{
			MessageID:     m.GetMessageId(),
			SenderUserID:  m.GetSenderUserId(),
			Text:          m.GetText(),
			CreatedAtUnix: m.GetCreatedAtUnix(),
		})
	}
	return messages, nil
}

func withBearerToken(ctx context.Context, bearerToken string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+bearerToken)
}
