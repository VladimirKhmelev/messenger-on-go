package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type txKey struct{}

type txState struct {
	tx             *sqlx.Tx
	enqueuedOutbox bool
}

func txFrom(ctx context.Context) *txState {
	st, _ := ctx.Value(txKey{}).(*txState)
	return st
}

func inTx(ctx context.Context) bool {
	return txFrom(ctx) != nil
}

func (r *PostgresChatRepository) db(ctx context.Context) sqlx.ExtContext {
	if st := txFrom(ctx); st != nil {
		return st.tx
	}
	return r.conn
}

func (r *PostgresChatRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if inTx(ctx) {
		return fn(ctx)
	}

	tx, err := r.conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	st := &txState{tx: tx}
	if err := fn(context.WithValue(ctx, txKey{}, st)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if st.enqueuedOutbox && r.onCommit != nil {
		r.onCommit()
	}
	return nil
}

func (r *PostgresChatRepository) OnCommit(fn func()) {
	r.onCommit = fn
}
