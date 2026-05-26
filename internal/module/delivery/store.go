package delivery

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// store is the delivery module's persistence boundary. The service depends on
// this interface so unit tests can swap in a fake. The pgstore implementation
// in pgstore.go owns the SQL.
type store interface {
	// nextContainerCode atomically draws the next per-day sequence and formats
	// CONT{YYYYMMDD}-{seq3}. Caller passes the wall clock so deterministic
	// tests can fix the date.
	nextContainerCode(ctx context.Context, now time.Time) (string, error)

	insertContainer(ctx context.Context, c Container) error
	selectContainerByID(ctx context.Context, id uuid.UUID) (Container, error)
	selectContainerLines(ctx context.Context, containerID uuid.UUID) ([]ContainerLine, error)
	selectContainersPaged(ctx context.Context, p httpkit.PageParams, f ContainerListFilter) ([]Container, int, error)
	selectStatusLog(ctx context.Context, containerID uuid.UUID) ([]ContainerStatusLogEntry, error)

	// withTx runs fn inside a single transaction. The closure receives both a
	// txStore (for delivery-owned tables) and the raw pgx.Tx so cross-module
	// hooks (sales.RecordShipmentTx during Seal) can run in the SAME tx as
	// the delivery writes. This is the deliberate exception to module
	// isolation; documented in deps.go.
	withTx(ctx context.Context, fn func(tx txStore, raw pgx.Tx) error) error
}

// txStore is the subset of operations safe to call from inside a transaction.
// Reads use SELECT ... FOR UPDATE so two concurrent transfer-line / seal
// requests serialize on the row.
type txStore interface {
	// lockContainerForUpdate locks the container row FOR UPDATE. Returns
	// ErrNotFound if the container does not exist.
	lockContainerForUpdate(ctx context.Context, id uuid.UUID) (Container, error)

	// lockLineForUpdate locks one container_lines row FOR UPDATE. Returns
	// ErrNotFound if the line does not exist.
	lockLineForUpdate(ctx context.Context, lineID uuid.UUID) (ContainerLine, error)

	// sumLinesAggregates returns the current cbm/weight totals for one
	// container, used by the AddLine / TransferLine guard. The aggregate is
	// computed inside the lock so it sees only committed sibling rows plus
	// the rows added in this very transaction.
	sumLinesAggregates(ctx context.Context, containerID uuid.UUID) (cbm, weight float64, err error)

	// listLinesForSeal returns all (sales_order_line_id, sum_qty) tuples for a
	// container. Used by Seal to drive the cross-module qty_shipped bump.
	listLinesForSeal(ctx context.Context, containerID uuid.UUID) ([]ShipmentItem, error)

	insertLine(ctx context.Context, line ContainerLine) error
	deleteLine(ctx context.Context, lineID uuid.UUID) error
	updateLineQty(ctx context.Context, lineID uuid.UUID, qty int, cbm, weight float64) error

	// updateContainerStatus flips status + records timestamps and writes the
	// matching container_status_log row in the same statement. Returns the
	// updated container.
	updateContainerStatus(ctx context.Context, in updateStatusInput) (Container, error)
}

type updateStatusInput struct {
	ContainerID uuid.UUID
	FromStatus  string
	ToStatus    string
	ActorID     uuid.UUID
	Note        string
	Now         time.Time
}
