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

	// ── Loading plans (#301) ─────────────────────────────────────────────────

	// insertLoadingPlanWithLines inserts the plan + every line atomically.
	// The unique partial index on (container_id, excel_hash) WHERE status
	// <> 'SUPERSEDED' makes BR-D10 race-proof — a duplicate hash returns
	// ErrInvalidInput. Lines never land without their parent.
	insertLoadingPlanWithLines(ctx context.Context, plan LoadingPlan, lines []LoadingPlanLine) error

	selectLoadingPlanByID(ctx context.Context, id uuid.UUID) (LoadingPlan, error)
	selectLoadingPlanLines(ctx context.Context, planID uuid.UUID) ([]LoadingPlanLine, error)

	// selectActiveLoadingPlan returns the latest non-SUPERSEDED plan for a
	// container (highest version). Returns ErrNotFound when no row matches.
	selectActiveLoadingPlan(ctx context.Context, containerID uuid.UUID) (LoadingPlan, error)

	// nextLoadingPlanVersion returns max(version)+1 for a container, or 1
	// when none exist. Caller must already hold a row lock to keep two
	// concurrent uploads from picking the same number.
	nextLoadingPlanVersion(ctx context.Context, containerID uuid.UUID) (int, error)

	// approveLoadingPlanTx flips status PARSED|VALIDATED -> APPROVED on
	// planID and SUPERSEDED on prior active plans of the same container.
	// Returns the updated plan row. Returns ErrInvalidTransition when the
	// plan is already SUPERSEDED.
	approveLoadingPlanTx(ctx context.Context, planID, actorID uuid.UUID, now time.Time) (LoadingPlan, error)

	// supersedeLoadingPlanTx atomically (a) snapshots every container_lines
	// row to container_lines_history under the named new plan, (b) DELETEs
	// container_lines for the container, (c) flips the prior active plan to
	// SUPERSEDED, (d) flips the new plan to APPROVED, and (e) returns the
	// updated new-plan row plus the count of lines that were wiped.
	//
	// Caller MUST have already verified container is not SEALED/SHIPPED
	// (BR-D12) — this method does not check status. Returns
	// ErrInvalidTransition when newPlanID is already SUPERSEDED, or
	// ErrNotFound when newPlanID does not exist.
	supersedeLoadingPlanTx(ctx context.Context, newPlanID, actorID uuid.UUID, now time.Time) (plan LoadingPlan, supersededCount int, err error)

	// countContainerLines returns the number of live container_lines rows for
	// one container. Used by the approve guard to decide whether the caller
	// must pass confirm_supersede=true (BR-D11).
	countContainerLines(ctx context.Context, containerID uuid.UUID) (int, error)

	// selectContainerLinesHistory returns audit rows for one container,
	// optionally filtered by the plan that triggered the supersede. Newest
	// row first.
	selectContainerLinesHistory(ctx context.Context, containerID uuid.UUID, planID *uuid.UUID) ([]ContainerLineHistoryEntry, error)

	// selectShortagesForContainer joins the active loading_plan_lines (planned)
	// against the current container_lines aggregate (actual loaded) and
	// returns one row per SKU where planned > actual. Used by Seal to drive
	// SHORT_SHIPPED auto-creation (BR-D15). Returns an empty report (no
	// active plan id) when the container has no non-SUPERSEDED plan.
	selectShortagesForContainer(ctx context.Context, containerID uuid.UUID) (ShortageReport, error)
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
