// Package postgres contains pgx-backed database adapters and keeps driver
// details out of feature packages.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type Session struct {
	pool *pgxpool.Pool
}

func OpenSession(ctx context.Context, config *pgxpool.Config) (*Session, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, core.NewError("connect", core.ErrorNetwork, "could not create the PostgreSQL connection pool", true, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, core.NewError("connect", core.ErrorNetwork, "could not reach PostgreSQL", true, err)
	}
	return &Session{pool: pool}, nil
}

func (s *Session) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return core.NewError("ping", core.ErrorNetwork, "PostgreSQL did not respond", true, err)
	}
	return nil
}

func (s *Session) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}
