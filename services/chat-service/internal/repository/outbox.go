package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

var errOutboxOutsideTx = errors.New("outbox: enqueue must run inside WithTx")

func (r *PostgresChatRepository) EnqueueOutbox(ctx context.Context, subject string, payload []byte, headers map[string][]string) error {
	st := txFrom(ctx)
	if st == nil {
		return errOutboxOutsideTx
	}
	st.enqueuedOutbox = true
	if headers == nil {
		headers = map[string][]string{}
	}
	encodedHeaders, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	_, err = r.db(ctx).ExecContext(ctx, `
		INSERT INTO outbox (subject, payload, headers) VALUES ($1, $2, $3)`,
		subject, payload, encodedHeaders,
	)
	return err
}

type outboxRow struct {
	ID      int64  `db:"id"`
	Subject string `db:"subject"`
	Payload []byte `db:"payload"`
	Headers []byte `db:"headers"`
}

func (r *PostgresChatRepository) RelayOutbox(ctx context.Context, limit int, publish func(ctx context.Context, msg domain.OutboxMessage) error) (int, error) {
	published := 0
	var publishErr error
	err := r.WithTx(ctx, func(ctx context.Context) error {
		published = 0
		var rows []outboxRow
		if err := sqlx.SelectContext(ctx, r.db(ctx), &rows, `
			SELECT id, subject, payload, headers FROM outbox
			ORDER BY id
			LIMIT $1
			FOR UPDATE SKIP LOCKED`,
			limit,
		); err != nil {
			return err
		}

		for _, row := range rows {
			msg := domain.OutboxMessage{ID: row.ID, Subject: row.Subject, Payload: row.Payload}
			if err := json.Unmarshal(row.Headers, &msg.Headers); err != nil {
				return err
			}
			if publishErr = publish(ctx, msg); publishErr != nil {
				// commit what was already published; keep this event and the
				// rest (in order) for the next attempt
				return nil
			}
			if _, err := r.db(ctx).ExecContext(ctx, `DELETE FROM outbox WHERE id = $1`, row.ID); err != nil {
				return err
			}
			published++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return published, publishErr
}
