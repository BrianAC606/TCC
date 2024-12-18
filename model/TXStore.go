package model

import (
	"TCC/pkg"
	"context"
	"time"
)

type TXStore interface {
	CreateTX(ctx context.Context, components ...TCCComponent) (string, error)
	TXUpdate(ctx context.Context, TXId string, componentId string, successful bool) error
	TXSubmit(ctx context.Context, TXId string, successful bool) error
	GetHangingTXs(context.Context) ([]*pkg.Transaction, error)
	GetTX(ctx context.Context, TXId string) (pkg.Transaction, error)
	Lock(ctx context.Context, duration time.Duration) error
	Unlock(ctx context.Context) error
}
