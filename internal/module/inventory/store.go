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
