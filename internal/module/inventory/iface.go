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
	Dimensions          domain.Dimension `json:"dimensions"`
	CostPerSheet        domain.Money     `json:"cost_per_sheet"`
	Status              string           `json:"status"`
	IssuedToWorkOrderID *uuid.UUID       `json:"issued_to_work_order_id,omitempty"`
	SupplierCode        *string          `json:"supplier_code,omitempty"`
	LotBatch            *string          `json:"lot_batch,omitempty"`
	GrainPattern        *string          `json:"grain_pattern,omitempty"`
	QualityGrade        *string          `json:"quality_grade,omitempty"`
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
	BoundingBoxLengthMM *int `json:"bounding_box_length_mm,omitempty"`
	BoundingBoxWidthMM  *int `json:"bounding_box_width_mm,omitempty"`
}

type CutResult struct {
	CuttingRecordID uuid.UUID  `json:"cutting_record_id"`
	RemnantID       *uuid.UUID `json:"remnant_id,omitempty"`
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
	AllocatedToWO       *uuid.UUID           `json:"allocated_to_wo,omitempty"`
	SupplierCode        *string              `json:"supplier_code,omitempty"`
	LotBatch            *string              `json:"lot_batch,omitempty"`
	GrainPattern        *string              `json:"grain_pattern,omitempty"`
	QualityGrade        *string              `json:"quality_grade,omitempty"`
	BoundingBoxLengthMM *int                 `json:"bounding_box_length_mm,omitempty"`
	BoundingBoxWidthMM  *int                 `json:"bounding_box_width_mm,omitempty"`
	BinLocationID       *uuid.UUID           `json:"bin_location_id,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
}

type Service interface {
	ReceiveStock(ctx context.Context, in ReceiveStockInput) (InventoryLot, error)
	ListLots(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[InventoryLot], error)
	DeactivateLot(ctx context.Context, lotID uuid.UUID) error

	GetSheet(ctx context.Context, sheetID uuid.UUID) (BoardSheet, error)
	ListAvailableSheets(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[BoardSheet], error)

	RecordCut(ctx context.Context, in RecordCutInput) (CutResult, error)

	// ListRemnants returns a paginated list of remnants matching the filter.
	// Status defaults to AVAILABLE when filter.Status is empty.
	ListRemnants(ctx context.Context, f RemnantFilter, p httpkit.PageParams) (httpkit.PagedResult[Remnant], error)
	// FindAvailableRemnants returns the full sorted list of AVAILABLE remnants that
	// meet the minimum bounding-box dimension. Used by the Best Fit algorithm.
	FindAvailableRemnants(ctx context.Context, minDim domain.Dimension) ([]Remnant, error)
	AllocateRemnant(ctx context.Context, remnantID uuid.UUID, workOrderID uuid.UUID) error
	MarkRemnantWaste(ctx context.Context, remnantID uuid.UUID) error
	GetRemnantLineage(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error)
	// GetRemnantLineageByRemnant resolves the parent_board_id from the given
	// remnant, then returns all remnants in the same lineage tree.
	GetRemnantLineageByRemnant(ctx context.Context, remnantID uuid.UUID) ([]Remnant, error)

	// ListStorageLocations returns all active storage locations.
	ListStorageLocations(ctx context.Context) ([]StorageLocation, error)
}
