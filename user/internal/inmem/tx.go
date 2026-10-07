package inmem

import "context"

// TxManager runs fn as it is: the in-memory store has no transactions, so
// what fn saved before a failure stays.
type TxManager struct{}

func (TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
