package common

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// SetTenantRLS sets app.tenant_id for the current PostgreSQL session/transaction.
// The `true` parameter makes it local to the current transaction.
func SetTenantRLS(ctx context.Context, db *sqlx.DB, tenantID string) error {
	_, err := db.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID)
	return err
}

// WithTenantRLS executes a function within a transaction that has app.tenant_id set.
// This ensures RLS policies apply correctly for the given tenant.
func WithTenantRLS(ctx context.Context, db *sqlx.DB, tenantID string, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}
