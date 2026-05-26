package packing

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// store is the packing module's repository contract. Unexported — only
// service.go binds to it; pgstore.go provides the production implementation.
type store interface {
	insertFGBatch(ctx context.Context, rows []FGPool) error
	selectFGByID(ctx context.Context, id uuid.UUID) (FGPool, error)
	selectFGByBarcodeID(ctx context.Context, barcodeID uuid.UUID) (FGPool, error)
	selectFGByWorkOrderID(ctx context.Context, woID uuid.UUID) ([]FGPool, error)
	selectFGPaged(ctx context.Context, p httpkit.PageParams, f FGListFilter) ([]FGPool, int, error)

	selectDefectByID(ctx context.Context, id uuid.UUID) (FGDefect, error)
	selectDefectByFGID(ctx context.Context, fgID uuid.UUID) (FGDefect, error)

	withTx(ctx context.Context, fn func(tx txStore) error) error
}

// txStore is the per-transaction surface used by service flows that need
// row locks (Reserve / Release / MarkLoaded / ReportDefect / ResolveDefect).
type txStore interface {
	lockFGForUpdate(ctx context.Context, id uuid.UUID) (FGPool, error)
	lockFGByBarcodeForUpdate(ctx context.Context, barcodeID uuid.UUID) (FGPool, error)
	lockAvailableFGsForReserve(ctx context.Context, skuID, soLineID uuid.UUID, qty int) ([]FGPool, error)
	lockReservedFGsByContainerLine(ctx context.Context, containerLineID uuid.UUID) ([]FGPool, error)
	lockReservedFGsByContainer(ctx context.Context, containerID uuid.UUID) ([]FGPool, error)

	flipFGStatus(ctx context.Context, in flipStatusInput) error
	bulkFlipFGStatus(ctx context.Context, ids []uuid.UUID, toStatus string, containerLineID *uuid.UUID) error

	insertDefect(ctx context.Context, d FGDefect) error
	updateDefectResolution(ctx context.Context, in updateResolutionInput) error

	rawTx() pgx.Tx
}

type flipStatusInput struct {
	FGID            uuid.UUID
	ToStatus        string
	ContainerLineID *uuid.UUID // nil clears the column
}

type updateResolutionInput struct {
	DefectID   uuid.UUID
	Resolution string
	Note       string
	ResolvedBy uuid.UUID
}
