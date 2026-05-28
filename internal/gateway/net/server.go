package net

import "context"

type NetworkServer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
