package loading_exception

import (
	"context"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// store is the loading_exception module's repository contract. Unexported —
// only service.go binds to it; pgstore.go provides the production
// implementation. Approve flips run inside withTx so the carry-over insert
// (BR-D17) and the row update commit atomically.
type store interface {
	insert(ctx context.Context, e LoadingException) error

	selectByID(ctx context.Context, id uuid.UUID) (LoadingException, error)

	selectByContainerKeyset(ctx context.Context, containerID uuid.UUID, status string, cur httpkit.Cursor, limit int) ([]LoadingException, error)

	// pendingByContainer returns the (count, ids) tuple for the BR-D14
	// SEAL pre-check. Backed by idx_le_pending so the query stays cheap
	// even when the container has many resolved exceptions in history.
	pendingByContainer(ctx context.Context, containerID uuid.UUID) (PendingSummary, error)

	withTx(ctx context.Context, fn func(tx txStore) error) error
}

// txStore is the per-transaction surface used by Approve. Locks the row,
// applies the resolution, and is the seam where the CarryOverCreator dep
// runs — it has to share the tx so the carry_over_so_line_id stamp and
// the new sales_order_lines insert commit together.
type txStore interface {
	lockForUpdate(ctx context.Context, id uuid.UUID) (LoadingException, error)
	approve(ctx context.Context, in approveRow) error
	reject(ctx context.Context, in rejectRow) error
}

type approveRow struct {
	ID                uuid.UUID
	ApprovedBy        uuid.UUID
	Resolution        string
	ResolutionNotes   string
	CarryOverSOLineID *uuid.UUID
	SubstituteSKUID   *uuid.UUID
}

type rejectRow struct {
	ID         uuid.UUID
	ApprovedBy uuid.UUID
	Reason     string
}
