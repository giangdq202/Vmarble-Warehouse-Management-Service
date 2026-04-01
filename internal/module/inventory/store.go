package inventory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertLot(ctx context.Context, lot InventoryLot) error
	selectLots(ctx context.Context) ([]InventoryLot, error)
	selectLotsPaged(ctx context.Context, p httpkit.PageParams) ([]InventoryLot, int, error)

	insertSheets(ctx context.Context, sheets []BoardSheet) error
	selectSheetByID(ctx context.Context, id uuid.UUID) (BoardSheet, error)
	selectAvailableSheets(ctx context.Context) ([]BoardSheet, error)
	selectAvailableSheetsPaged(ctx context.Context, p httpkit.PageParams) ([]BoardSheet, int, error)
	updateSheetStatus(ctx context.Context, id uuid.UUID, status string, issuedToWO *uuid.UUID) error

	insertCuttingRecord(ctx context.Context, cr CuttingRecord) error

	insertRemnant(ctx context.Context, r Remnant) error
	selectAvailableRemnantsByMinDimension(ctx context.Context, minDim domain.Dimension) ([]Remnant, error)
	// selectRemnantsByFilter returns a paginated slice of remnants matching the
	// filter, plus the total count of matching rows.
	selectRemnantsByFilter(ctx context.Context, f RemnantFilter, p httpkit.PageParams) ([]Remnant, int, error)
	selectRemnantsByBoardSheet(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error)
	selectRemnantByID(ctx context.Context, id uuid.UUID) (Remnant, error)
	updateRemnantStatus(ctx context.Context, id uuid.UUID, status domain.RemnantStatus, allocatedToWO *uuid.UUID) error

	// selectActiveStorageLocations returns all storage locations where is_active = true.
	selectActiveStorageLocations(ctx context.Context) ([]StorageLocation, error)

	// recordCutAtomically executes all cut-related writes inside a single DB
	// transaction. It acquires a row-level lock (SELECT … FOR UPDATE) on the
	// source sheet or remnant first, re-validates the status under the lock to
	// prevent double-allocation, then applies the writes atomically.
	// BR-K03 area-conservation must still be validated by the caller (service)
	// before calling this, using the unlocked read that happens in RecordCut.
	recordCutAtomically(ctx context.Context, op cutWriteOp) error

	// allocateRemnantAtomically acquires a row-level lock on the remnant, checks
	// that it is still AVAILABLE, and transitions it to ALLOCATED — all in one
	// transaction. Returns ErrPreconditionFailed if the remnant is no longer
	// available by the time the lock is acquired.
	allocateRemnantAtomically(ctx context.Context, remnantID uuid.UUID, workOrderID uuid.UUID) error

	// markRemnantWasteAtomically acquires a row-level lock on the remnant, checks
	// that it is in a wasteable state (AVAILABLE or ALLOCATED), and transitions
	// it to WASTE — all in one transaction. Returns ErrPreconditionFailed if the
	// remnant is in a non-wasteable status when the lock is acquired.
	markRemnantWasteAtomically(ctx context.Context, remnantID uuid.UUID) error
}

// cutWriteOp carries the pre-validated, ready-to-persist data for a single cut
// operation. Exactly one of SheetUpdate / RemnantUpdate must be non-nil.
type cutWriteOp struct {
	// Record is the cutting_record row to insert.
	Record CuttingRecord

	// SheetUpdate is set when the cut source is a board sheet.
	SheetUpdate *sheetStatusUpdate
	// RemnantUpdate is set when the cut source is a remnant.
	RemnantUpdate *remnantStatusUpdate

	// NewRemnant is set when the cut produces a leftover remnant.
	NewRemnant *Remnant
}

type sheetStatusUpdate struct {
	ID         uuid.UUID
	Status     string
	IssuedToWO *uuid.UUID
}

type remnantStatusUpdate struct {
	ID     uuid.UUID
	Status domain.RemnantStatus
}

type CuttingRecord struct {
	ID              uuid.UUID  `json:"id"`
	SheetID         *uuid.UUID `json:"sheet_id,omitempty"`
	RemnantSourceID *uuid.UUID `json:"remnant_source_id,omitempty"`
	WorkOrderID     uuid.UUID  `json:"work_order_id"`
	SKUID           uuid.UUID  `json:"sku_id"`
	UsedLengthMM    int        `json:"used_length_mm"`
	UsedWidthMM     int        `json:"used_width_mm"`
	CreatedAt       time.Time  `json:"created_at"`
}
