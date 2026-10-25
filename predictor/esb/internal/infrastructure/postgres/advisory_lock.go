package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const advisoryLockReleaseTimeout = 2 * time.Second

func releaseAdvisoryLock(ctx context.Context, conn *pgxpool.Conn, query string, args ...any) error {
	unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), advisoryLockReleaseTimeout)
	defer cancel()
	if _, err := conn.Exec(unlockCtx, query, args...); err == nil {
		conn.Release()
		return nil
	} else {
		connection := conn.Hijack()
		closeErr := connection.Close(unlockCtx)
		if closeErr != nil {
			return errors.Join(fmt.Errorf("unlock PostgreSQL advisory lock: %w", err), fmt.Errorf("close locked PostgreSQL connection: %w", closeErr))
		}
		return fmt.Errorf("unlock PostgreSQL advisory lock: %w", err)
	}
}

func discardPoolConnection(ctx context.Context, conn *pgxpool.Conn, cause error) error {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), advisoryLockReleaseTimeout)
	defer cancel()
	closeErr := conn.Hijack().Close(closeCtx)
	if closeErr != nil {
		return errors.Join(cause, fmt.Errorf("close PostgreSQL connection: %w", closeErr))
	}
	return cause
}
