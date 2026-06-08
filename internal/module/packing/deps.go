package packing

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// BarcodeIssuer generates a barcode for a freshly QC'd FG. Wired in main.go
// to barcode.Service.GenerateBarcode. The hook owns one barcode-per-FG so
// the kiosk scan flow can resolve back to a unique fg_pool row.
type BarcodeIssuer interface {
	GenerateBarcode(ctx context.Context, in BarcodeIssueInput) (BarcodeRef, error)
}

type BarcodeIssueInput struct {
	WorkOrderID      uuid.UUID
	SKUID            uuid.UUID
	POID             uuid.UUID
	ProductionPlanID uuid.UUID
	SKUCode          string
	SKUName          string
	Dimensions       string
	ProducedDate     time.Time
}

type BarcodeRef struct {
	ID uuid.UUID
}

// BarcodeResolver looks up an existing barcode at scan time and returns the
// FG-relevant projection. Wired to barcode.Service.LookupBarcode in main.go.
type BarcodeResolver interface {
	LookupBarcode(ctx context.Context, barcodeID uuid.UUID) (BarcodeLookup, error)
}

type BarcodeLookup struct {
	ID          uuid.UUID
	WorkOrderID uuid.UUID
	SKUID       uuid.UUID
}

// WorkOrderGateway returns the WO status so ScanBarcode can enforce BR-PK01
// (scan only valid when WO.status = COMPLETED). The slim projection avoids
// pulling the entire production WO struct into the packing module.
type WorkOrderGateway interface {
	GetWorkOrderStatus(ctx context.Context, woID uuid.UUID) (WorkOrderStatusInfo, error)
}

type WorkOrderStatusInfo struct {
	ID     uuid.UUID
	Status string
}

// ContainerSuggester returns OPEN/LOADING containers carrying the same SO
// line, ordered by fill_pct asc so the operator picks the most-empty one
// first. Wired to a delivery-module adapter in main.go.
type ContainerSuggester interface {
	SuggestForSOLine(ctx context.Context, soLineID uuid.UUID) ([]ContainerSuggestion, error)
}

// ContainerLineRemover lets ReportDefect strip the FG from its container
// line when the defective FG was already RESERVED. Wired to delivery via
// the same Service surface delivery exposes for line operations.
type ContainerLineRemover interface {
	DeleteLineForDefect(ctx context.Context, containerLineID uuid.UUID, actorID uuid.UUID) error
}

// DefectNotifier fans out fg.defect.created / fg.defect.resolved SSE events.
// Best-effort: errors are logged, never fail the request.
type DefectNotifier interface {
	NotifyFGDefect(ctx context.Context, fgID uuid.UUID, skuCode, reason string) error
	NotifyFGDefectResolved(ctx context.Context, fgID uuid.UUID, resolution string) error
}

// SOLineChecker returns the slim SOL projection packing needs to validate a
// reassignment target. Wired in main.go to sales.Service.GetSOLine.
type SOLineChecker interface {
	GetSOLine(ctx context.Context, soLineID uuid.UUID) (SOLineInfo, error)
}

type SOLineInfo struct {
	ID    uuid.UUID
	SKUID uuid.UUID
}

// SKUComponentResolver fetches the multi-component breakdown for a SKU.
// Used by CreateFromCompletedWO to create one FG row per component per unit.
// When a SKU has no components, the result is empty and the caller falls back
// to single-row-per-unit behaviour (simple / single-box SKU).
// Wired in main.go to catalog.Service.ListSKUComponents.
type SKUComponentResolver interface {
	GetSKUComponents(ctx context.Context, skuID uuid.UUID) ([]SKUComponentInfo, error)
}

type SKUComponentInfo struct {
	ComponentType string
	CbmPerUnit    float64
	SortOrder     int
}
