// Package postgres contains pgx-backed database adapters and keeps driver
// details out of feature packages.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Session struct {
	pool *pgxpool.Pool
}

func OpenSession(ctx context.Context, config *pgxpool.Config) (*Session, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, ClassifyError("connect", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, ClassifyError("connect", err)
	}
	return &Session{pool: pool}, nil
}

func (s *Session) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return ClassifyError("ping", err)
	}
	return nil
}

func (s *Session) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}
