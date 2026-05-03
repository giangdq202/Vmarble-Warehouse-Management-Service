package inventory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type ReceiveStockInput struct {
	MaterialID   uuid.UUID        `json:"material_id"`
	Dimensions   domain.Dimension `json:"dimensions"`
	CostPerSheet domain.Money     `json:"cost_per_sheet"`
	Quantity     int              `json:"quantity"`
	SupplierRef  string           `json:"supplier_ref"`
}

type InventoryLot struct {
	ID           uuid.UUID    `json:"id"`
	MaterialID   uuid.UUID    `json:"material_id"`
	Quantity     int          `json:"quantity"`
	CostPerSheet domain.Money `json:"cost_per_sheet"`
	SupplierRef  string       `json:"supplier_ref"`
	IsActive     bool         `json:"is_active"`
	ReceivedAt   time.Time    `json:"received_at"`
}

type BoardSheet struct {
	ID                  uuid.UUID        `json:"id"`
	LotID               uuid.UUID        `json:"lot_id"`
	MaterialID          uuid.UUID        `json:"material_id"`
	MaterialName        string           `json:"material_name"`
	Dimensions          domain.Dimension `json:"dimensions"`
	CostPerSheet        domain.Money     `json:"cost_per_sheet"`
	Status              string           `json:"status"`
	IssuedToWorkOrderID *uuid.UUID       `json:"issued_to_work_order_id,omitempty"`
	SupplierCode        *string          `json:"supplier_code,omitempty"`
	LotBatch            *string          `json:"lot_batch,omitempty"`
	GrainPattern        *string          `json:"grain_pattern,omitempty"`
	QualityGrade        *string          `json:"quality_grade,omitempty"`
	BinLocationID       *uuid.UUID       `json:"bin_location_id,omitempty"`
}

type RecordCutInput struct {
	SheetID          *uuid.UUID        `json:"sheet_id,omitempty"`
	RemnantID        *uuid.UUID        `json:"remnant_id,omitempty"`
	WorkOrderID      uuid.UUID         `json:"work_order_id"`
	SKUID            uuid.UUID         `json:"sku_id"`
	UsedDimension    domain.Dimension  `json:"used_dimension"`
	RemnantDimension *domain.Dimension `json:"remnant_dimension,omitempty"`
	// BoundingBoxLengthMM and BoundingBoxWidthMM define the usable area of the
	// new remnant produced by this cut (e.g. after a chipped corner is excluded).
	// Both must be provided together. If omitted, the system defaults to the
	// actual remnant dimension so that search queries always have a value to
	// filter on. Must not exceed the corresponding RemnantDimension axis.
	BoundingBoxLengthMM *int   `json:"bounding_box_length_mm,omitempty"`
	BoundingBoxWidthMM  *int   `json:"bounding_box_width_mm,omitempty"`
	ShapeType           string `json:"shape_type,omitempty"` // "rectangle" (default) | "irregular"
}

type CutResult struct {
	CuttingRecordID uuid.UUID   `json:"cutting_record_id"`
	RemnantID       *uuid.UUID  `json:"remnant_id,omitempty"`
	BarcodeIDs      []uuid.UUID `json:"barcode_ids,omitempty"`
}

// RemnantFilter holds optional filter parameters for ListRemnants.
// Zero values mean "no filter" for that field.
type RemnantFilter struct {
	// MinLengthMM filters by COALESCE(bounding_box_length_mm, length_mm) >= MinLengthMM.
	// 0 means no lower-bound filter on length.
	MinLengthMM int
	// MinWidthMM filters by COALESCE(bounding_box_width_mm, width_mm) >= MinWidthMM.
	// 0 means no lower-bound filter on width.
	MinWidthMM int
	// Status restricts results to a specific remnant status.
	// Defaults to AVAILABLE when empty.
	Status domain.RemnantStatus
}

// StorageLocation represents a physical shelf / bin where remnants are stored.
type StorageLocation struct {
	ID        uuid.UUID `json:"id"`
	Zone      string    `json:"zone"`
	Rack      string    `json:"rack"`
	Shelf     string    `json:"shelf"`
	Label     string    `json:"label"`
	Barcode   string    `json:"barcode"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type Remnant struct {
	ID                  uuid.UUID            `json:"id"`
	ParentBoardID       uuid.UUID            `json:"parent_board_id"`
	ParentRemnantID     *uuid.UUID           `json:"parent_remnant_id,omitempty"`
	Dimensions          domain.Dimension     `json:"dimensions"`
	Status              domain.RemnantStatus `json:"status"`
	ShapeType           string               `json:"shape_type"`
	AllocatedToWO       *uuid.UUID           `json:"allocated_to_wo,omitempty"`
	AllocatedAt         *time.Time           `json:"allocated_at,omitempty"`
	SupplierCode        *string              `json:"supplier_code,omitempty"`
	LotBatch            *string              `json:"lot_batch,omitempty"`
	GrainPattern        *string              `json:"grain_pattern,omitempty"`
	QualityGrade        *string              `json:"quality_grade,omitempty"`
	BoundingBoxLengthMM *int                 `json:"bounding_box_length_mm,omitempty"`
	BoundingBoxWidthMM  *int                 `json:"bounding_box_width_mm,omitempty"`
	BinLocationID       *uuid.UUID           `json:"bin_location_id,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

// RemnantSuggestion pairs a candidate remnant with its physical storage
// location (if stocked) and a 1-based rank. Suggestions are ordered by the
// Best Fit + FIFO algorithm: smallest bounding-box area first, oldest first
// among ties.
type RemnantSuggestion struct {
	Remnant  Remnant          `json:"remnant"`
	Location *StorageLocation `json:"location,omitempty"`
	Rank     int              `json:"rank"`
}

// SuggestRemnantsInput carries the parameters for the Best Fit + FIFO
// remnant suggestion query.
type SuggestRemnantsInput struct {
	RequiredDimension domain.Dimension
	Limit             int // defaults to 3; clamped to [1, 10]
}

type RemnantLabelSize string

const (
	RemnantLabelSize50x30  RemnantLabelSize = "50x30"
	RemnantLabelSize100x70 RemnantLabelSize = "100x70"
)

// RemnantLabelInput carries the parameters for generating a remnant stock label.
type RemnantLabelInput struct {
	RemnantID uuid.UUID
	Size      RemnantLabelSize
}

type OverflowLevel string

const (
	OverflowGreen OverflowLevel = "GREEN"
	OverflowRed   OverflowLevel = "RED"
)

type OverflowStatus struct {
	Status              OverflowLevel `json:"status"`
	OverflowPct         float64       `json:"overflow_pct"`
	ThresholdPct        float64       `json:"threshold_pct"`
	BlockNewSheetIssue  bool          `json:"block_new_sheet_issue"`
	TotalRemnantAreaMM2 int64         `json:"total_remnant_area_mm2"`
	TotalSheetAreaMM2   int64         `json:"total_sheet_area_mm2"`
}

// TransferInput carries a request to move a Remnant or BoardSheet to a new bin location.
type TransferInput struct {
	EntityType    string    `json:"entity_type"`
	EntityID      uuid.UUID `json:"entity_id"`
	TargetBarcode string    `json:"target_location_barcode"`
	ActorID       uuid.UUID `json:"-"`
}

// TransferResult is the response after a successful stock transfer.
type TransferResult struct {
	EntityType   string     `json:"entity_type"`
	EntityID     uuid.UUID  `json:"entity_id"`
	FromLocation *uuid.UUID `json:"from_location,omitempty"`
	ToLocation   uuid.UUID  `json:"to_location"`
	AuditLogID   uuid.UUID  `json:"audit_log_id"`
}

// AuditLogEntry records a single inventory change event (transfer or cycle count adjustment).
type AuditLogEntry struct {
	ID           uuid.UUID  `json:"id"`
	EntityType   string     `json:"entity_type"`
	EntityID     uuid.UUID  `json:"entity_id"`
	Action       string     `json:"action"`
	ActorID      uuid.UUID  `json:"actor_id"`
	FromLocation *uuid.UUID `json:"from_location,omitempty"`
	ToLocation   *uuid.UUID `json:"to_location,omitempty"`
	FromStatus   *string    `json:"from_status,omitempty"`
	ToStatus     *string    `json:"to_status,omitempty"`
	Reason       *string    `json:"reason,omitempty"`
	SessionID    *uuid.UUID `json:"session_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// CreateCycleCountInput carries a request to open a new cycle count session.
type CreateCycleCountInput struct {
	Zone    string    `json:"zone,omitempty"`
	ActorID uuid.UUID `json:"-"`
}

// CycleCountSession represents a cycle count session with its lifecycle status.
type CycleCountSession struct {
	ID        uuid.UUID  `json:"id"`
	Zone      string     `json:"zone,omitempty"`
	Status    string     `json:"status"`
	CreatedBy uuid.UUID  `json:"created_by"`
	PostedBy  *uuid.UUID `json:"posted_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	PostedAt  *time.Time `json:"posted_at,omitempty"`
}

// AddCountLineInput carries a single physical count observation for a session.
type AddCountLineInput struct {
	SessionID         uuid.UUID  `json:"-"`
	EntityType        string     `json:"entity_type"`
	EntityID          uuid.UUID  `json:"entity_id"`
	CountedStatus     string     `json:"counted_status"`
	CountedLocationID *uuid.UUID `json:"counted_location_id,omitempty"`
	Reason            string     `json:"reason"`
}

// CycleCountLine represents one counted item within a cycle count session.
type CycleCountLine struct {
	ID                uuid.UUID  `json:"id"`
	SessionID         uuid.UUID  `json:"session_id"`
	EntityType        string     `json:"entity_type"`
	EntityID          uuid.UUID  `json:"entity_id"`
	CountedStatus     string     `json:"counted_status"`
	CountedLocationID *uuid.UUID `json:"counted_location_id,omitempty"`
	Reason            string     `json:"reason"`
	CreatedAt         time.Time  `json:"created_at"`
}

// PostCycleCountInput carries identifiers for posting a cycle count session.
type PostCycleCountInput struct {
	SessionID uuid.UUID `json:"-"`
	ActorID   uuid.UUID `json:"-"`
}

type Service interface {
	ReceiveStock(ctx context.Context, in ReceiveStockInput) (InventoryLot, error)
	ListLots(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[InventoryLot], error)
	DeactivateLot(ctx context.Context, lotID uuid.UUID) error

	GetSheet(ctx context.Context, sheetID uuid.UUID) (BoardSheet, error)
	// ListAvailableSheets returns AVAILABLE sheets, optionally filtered by materialID.
	ListAvailableSheets(ctx context.Context, p httpkit.PageParams, materialID *uuid.UUID) (httpkit.PagedResult[BoardSheet], error)
	GetOverflowStatus(ctx context.Context) (OverflowStatus, error)
	// PreAssignSheet stamps issued_to_work_order_id on a board sheet that is still
	// AVAILABLE, reserving it for a work order before cutting begins.
	// Returns ErrPreconditionFailed if the sheet is not AVAILABLE.
	PreAssignSheet(ctx context.Context, sheetID uuid.UUID, workOrderID uuid.UUID) error

	RecordCut(ctx context.Context, in RecordCutInput) (CutResult, error)

	// ListRemnants returns a paginated list of remnants matching the filter.
	// Status defaults to AVAILABLE when filter.Status is empty.
	ListRemnants(ctx context.Context, f RemnantFilter, p httpkit.PageParams) (httpkit.PagedResult[Remnant], error)
	// GetRemnant returns a single remnant by ID.
	GetRemnant(ctx context.Context, remnantID uuid.UUID) (Remnant, error)
	// FindAvailableRemnants returns the full sorted list of AVAILABLE remnants that
	// meet the minimum bounding-box dimension. Used by the Best Fit algorithm.
	FindAvailableRemnants(ctx context.Context, minDim domain.Dimension) ([]Remnant, error)
	// SuggestRemnants returns the top-N AVAILABLE remnants that fit the required
	// dimension, ranked by Best Fit (smallest area) + FIFO (oldest first). Each
	// suggestion includes the remnant's storage location when available.
	SuggestRemnants(ctx context.Context, in SuggestRemnantsInput) ([]RemnantSuggestion, error)
	AllocateRemnant(ctx context.Context, remnantID uuid.UUID, workOrderID uuid.UUID) error
	MarkRemnantWaste(ctx context.Context, remnantID uuid.UUID) error
	// StockRemnant assigns a remnant to a physical storage bin by looking up the
	// location with the given barcode string. The remnant must be AVAILABLE.
	// Returns ErrNotFound if the barcode does not match any active storage location.
	StockRemnant(ctx context.Context, remnantID uuid.UUID, locationBarcode string) error
	GetRemnantLineage(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error)
	// GetRemnantLineageByRemnant resolves the parent_board_id from the given
	// remnant, then returns all remnants in the same lineage tree.
	GetRemnantLineageByRemnant(ctx context.Context, remnantID uuid.UUID) ([]Remnant, error)

	// ReleaseExpiredAllocations resets ALLOCATED remnants whose allocated_at
	// timestamp is older than `before` back to AVAILABLE. Returns the number
	// of remnants released. Used by the background auto-release task.
	ReleaseExpiredAllocations(ctx context.Context, before time.Time) (int, error)

	// ListStorageLocations returns all active storage locations.
	ListStorageLocations(ctx context.Context) ([]StorageLocation, error)

	// Transfer moves a Remnant or BoardSheet to a new bin location and writes an audit log entry.
	Transfer(ctx context.Context, in TransferInput) (TransferResult, error)
	// ListAuditLog returns audit log entries for a given entity ordered by created_at DESC.
	ListAuditLog(ctx context.Context, entityID uuid.UUID, entityType string) ([]AuditLogEntry, error)

	// CreateCycleCountSession opens a new OPEN cycle count session.
	CreateCycleCountSession(ctx context.Context, in CreateCycleCountInput) (CycleCountSession, error)
	// GetCycleCountSession returns a session by ID.
	GetCycleCountSession(ctx context.Context, sessionID uuid.UUID) (CycleCountSession, error)
	// AddCycleCountLine adds a physical count observation to an OPEN session.
	AddCycleCountLine(ctx context.Context, in AddCountLineInput) (CycleCountLine, error)
	// ListCycleCountLines returns all lines in a session.
	ListCycleCountLines(ctx context.Context, sessionID uuid.UUID) ([]CycleCountLine, error)
	// PostCycleCount applies adjustments from an OPEN session and transitions it to POSTED.
	PostCycleCount(ctx context.Context, in PostCycleCountInput) error
	// CancelCycleCountSession cancels an OPEN session without applying any changes.
	CancelCycleCountSession(ctx context.Context, sessionID uuid.UUID, actorID uuid.UUID) error

	// GenerateRemnantLabelPDF renders a compact stock label PDF for a remnant.
	// The label contains a QR code encoding the remnant identity and a short
	// text block with the remnant ID, parent board ID, and dimensions.
	// Size must be one of RemnantLabelSize50x30 or RemnantLabelSize100x70.
	GenerateRemnantLabelPDF(ctx context.Context, in RemnantLabelInput) ([]byte, error)
}
