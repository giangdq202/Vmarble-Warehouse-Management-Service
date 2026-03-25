package inventory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type store interface {
	insertLot(ctx context.Context, lot InventoryLot) error
	selectLots(ctx context.Context) ([]InventoryLot, error)

	insertSheets(ctx context.Context, sheets []BoardSheet) error
	selectSheetByID(ctx context.Context, id uuid.UUID) (BoardSheet, error)
	selectAvailableSheets(ctx context.Context) ([]BoardSheet, error)
	updateSheetStatus(ctx context.Context, id uuid.UUID, status string, issuedToWO *uuid.UUID) error

	insertCuttingRecord(ctx context.Context, cr CuttingRecord) error

	insertRemnant(ctx context.Context, r Remnant) error
	selectAvailableRemnantsByMinDimension(ctx context.Context, minDim domain.Dimension) ([]Remnant, error)
	selectRemnantsByBoardSheet(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error)
	selectRemnantByID(ctx context.Context, id uuid.UUID) (Remnant, error)
	updateRemnantStatus(ctx context.Context, id uuid.UUID, status domain.RemnantStatus, allocatedToWO *uuid.UUID) error

	// recordCutAtomically executes all cut-related writes inside a single DB
	// transaction: updating the source sheet/remnant status, inserting the
	// cutting record, and optionally inserting a new remnant.
	// The caller (service) must have already validated all business invariants
	// (BR-K03 area conservation, source status checks) before calling this.
	recordCutAtomically(ctx context.Context, op cutWriteOp) error
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
