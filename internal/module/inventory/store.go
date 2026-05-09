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
	deactivateLot(ctx context.Context, id uuid.UUID) error

	insertSheets(ctx context.Context, sheets []BoardSheet) error
	selectSheetByID(ctx context.Context, id uuid.UUID) (BoardSheet, error)
	selectAvailableSheets(ctx context.Context) ([]BoardSheet, error)
	selectAvailableSheetsPaged(ctx context.Context, p httpkit.PageParams, materialID *uuid.UUID) ([]BoardSheet, int, error)
	selectOverflowAreas(ctx context.Context) (int64, int64, error)
	updateSheetStatus(ctx context.Context, id uuid.UUID, status string, issuedToWO *uuid.UUID) error
	// preAssignSheet sets issued_to_work_order_id on an AVAILABLE sheet inside a
	// transaction with FOR UPDATE. Returns ErrPreconditionFailed if the sheet
	// is no longer AVAILABLE when the lock is acquired.
	preAssignSheet(ctx context.Context, sheetID uuid.UUID, workOrderID uuid.UUID) error

	insertCuttingRecord(ctx context.Context, cr CuttingRecord) error

	insertRemnant(ctx context.Context, r Remnant) error
	selectAvailableRemnantsByMinDimension(ctx context.Context, minDim domain.Dimension) ([]Remnant, error)
	// selectTopRemnantSuggestions returns up to `limit` AVAILABLE remnants whose
	// bounding box fits minDim, ranked by Best Fit (smallest area) + FIFO
	// (oldest created_at). Each result is LEFT JOINed with storage_locations.
	selectTopRemnantSuggestions(ctx context.Context, minDim domain.Dimension, limit int) ([]RemnantSuggestion, error)
	// selectRemnantsByFilter returns a paginated slice of remnants matching the
	// filter, plus the total count of matching rows.
	selectRemnantsByFilter(ctx context.Context, f RemnantFilter, p httpkit.PageParams) ([]Remnant, int, error)
	selectRemnantsByBoardSheet(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error)
	selectRemnantByID(ctx context.Context, id uuid.UUID) (Remnant, error)
	updateRemnantStatus(ctx context.Context, id uuid.UUID, status domain.RemnantStatus, allocatedToWO *uuid.UUID) error
	// updateRemnantBinLocation sets bin_location_id on the remnant row.
	updateRemnantBinLocation(ctx context.Context, remnantID uuid.UUID, locationID uuid.UUID) error

	// updateSheetBinLocation sets bin_location_id on the board sheet row.
	updateSheetBinLocation(ctx context.Context, sheetID uuid.UUID, locationID uuid.UUID) error

	// selectActiveStorageLocations returns all storage locations where is_active = true.
	selectActiveStorageLocations(ctx context.Context) ([]StorageLocation, error)
	// selectStorageLocationByBarcode returns the storage location whose barcode
	// field matches exactly (case-sensitive). Returns ErrNotFound if no match.
	selectStorageLocationByBarcode(ctx context.Context, barcode string) (StorageLocation, error)

	// insertAuditLog persists an inventory change audit event.
	insertAuditLog(ctx context.Context, entry AuditLogEntry) error
	// selectAuditLogByEntity returns audit entries for the given entity, newest first.
	selectAuditLogByEntity(ctx context.Context, entityID uuid.UUID, entityType string) ([]AuditLogEntry, error)
	// selectAuditLogByAction returns audit entries for the given action across all
	// entities, newest first.
	selectAuditLogByAction(ctx context.Context, action string) ([]AuditLogEntry, error)

	// insertCycleCountSession persists a new cycle count session row.
	insertCycleCountSession(ctx context.Context, s CycleCountSession) error
	// selectCycleCountSessionByID returns the session or ErrNotFound.
	selectCycleCountSessionByID(ctx context.Context, id uuid.UUID) (CycleCountSession, error)
	// updateCycleCountSessionStatus transitions the session status and optionally sets posted_by.
	updateCycleCountSessionStatus(ctx context.Context, id uuid.UUID, status string, postedBy *uuid.UUID) error
	// insertCycleCountLine adds a counted line to a session.
	// Returns ErrPreconditionFailed on duplicate (session_id, entity_type, entity_id).
	insertCycleCountLine(ctx context.Context, l CycleCountLine) error
	// selectCycleCountLinesBySession returns all lines for a session ordered by created_at.
	selectCycleCountLinesBySession(ctx context.Context, sessionID uuid.UUID) ([]CycleCountLine, error)
	// postCycleCountAtomically applies all adjustments and transitions the session to POSTED
	// inside a single transaction, using FOR UPDATE on the session row.
	postCycleCountAtomically(ctx context.Context, op cycleCountPostOp) error

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

	// releaseExpiredAllocations resets ALLOCATED remnants whose allocated_at is
	// older than `before` back to AVAILABLE (allocated_to_wo_id and allocated_at
	// are cleared). Returns the number of rows updated.
	releaseExpiredAllocations(ctx context.Context, before time.Time) (int64, error)
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

// cycleCountPostOp carries all adjustments to apply when posting a cycle count session.
type cycleCountPostOp struct {
	SessionID   uuid.UUID
	PostedBy    uuid.UUID
	Adjustments []cycleCountAdjustment
}

// cycleCountAdjustment describes one entity change produced by a cycle count line diff.
type cycleCountAdjustment struct {
	EntityType    string
	EntityID      uuid.UUID
	OldLocationID *uuid.UUID
	NewLocationID *uuid.UUID
	OldStatus     string
	NewStatus     string
	AuditEntry    AuditLogEntry
}
