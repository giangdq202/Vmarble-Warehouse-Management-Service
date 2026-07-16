package catalog

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type MaterialType string

const (
	MaterialTypePlywood MaterialType = "PLYWOOD"
	MaterialTypeGlue    MaterialType = "GLUE"
	MaterialTypeMetal   MaterialType = "METAL"
	MaterialTypeOther   MaterialType = "OTHER"
)

type Material struct {
	ID                 uuid.UUID    `json:"id"`
	Type               MaterialType `json:"type"`
	Name               string       `json:"name"`
	Unit               string       `json:"unit"`
	IsActive           bool         `json:"is_active"`
	MinRemnantLengthMM int          `json:"min_remnant_length_mm"`
	MinRemnantWidthMM  int          `json:"min_remnant_width_mm"`
	CreatedAt          time.Time    `json:"created_at"`
}

type CreateMaterialInput struct {
	Type MaterialType `json:"type"`
	Name string       `json:"name"`
	Unit string       `json:"unit"`
}

// UpdateMinRemnantPolicyInput carries new threshold values for BR-K06/K07/K08.
// Both axes must be non-negative integers in millimetres. A value of 0 disables
// enforcement on that axis.
type UpdateMinRemnantPolicyInput struct {
	MaterialID         uuid.UUID `json:"-"`
	MinRemnantLengthMM int       `json:"min_remnant_length_mm"`
	MinRemnantWidthMM  int       `json:"min_remnant_width_mm"`
	ActorID            uuid.UUID `json:"-"`
}

type SKU struct {
	ID            uuid.UUID        `json:"id"`
	Code          string           `json:"code"`
	Name          string           `json:"name"`
	Dimensions    domain.Dimension `json:"dimensions"`
	RequiresMetal bool             `json:"requires_metal"`
	IsActive      bool             `json:"is_active"`
	HeightMM      *int             `json:"height_mm,omitempty"`
	WeightKg      *float64         `json:"weight_kg,omitempty"`
	HSCode        *string          `json:"hs_code,omitempty"`
	CbmPerUnit    *float64         `json:"cbm_per_unit,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}

type CreateSKUInput struct {
	Code          string           `json:"code"`
	Name          string           `json:"name"`
	Dimensions    domain.Dimension `json:"dimensions"`
	RequiresMetal bool             `json:"requires_metal"`
}

// UpdateSKUInput carries export/shipping fields for PATCH /skus/:id.
// Nil pointer means "leave unchanged". HeightMM/WeightKg/HSCode are nullable
// in the DB (legacy SKUs keep NULL until explicitly set).
type UpdateSKUInput struct {
	SKUID    uuid.UUID
	HeightMM *int
	WeightKg *float64
	HSCode   *string
}

// SKUComponent represents one physical sub-package a SKU ships in.
// Examples: TOP, BASE, DRAWER. component_type is free-text; unique per SKU.
// cbm_per_unit is the individual package's volume in cubic metres.
type SKUComponent struct {
	ID            uuid.UUID `json:"id"`
	SKUID         uuid.UUID `json:"sku_id"`
	ComponentType string    `json:"component_type"`
	CbmPerUnit    float64   `json:"cbm_per_unit"`
	SortOrder     int       `json:"sort_order"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpsertSKUComponentInput creates or replaces one (sku_id, component_type) row.
// BR-PK-MULTI01: component_type must be a non-empty string unique per SKU.
type UpsertSKUComponentInput struct {
	SKUID         uuid.UUID `json:"sku_id"`
	ComponentType string    `json:"component_type"`
	CbmPerUnit    float64   `json:"cbm_per_unit"`
	SortOrder     int       `json:"sort_order"`
}

// PackingUnit represents one row in sku_packing_units.
// unit: piece | set | carton; pieces_per_unit >= 1.
type PackingUnit struct {
	SKUID         uuid.UUID `json:"sku_id"`
	Unit          string    `json:"unit"`
	PiecesPerUnit int       `json:"pieces_per_unit"`
	IsDefault     bool      `json:"is_default"`
}

// UpsertPackingUnitInput creates or replaces one (sku_id, unit) row.
// BR-SKU04: each SKU must have exactly one is_default=true row before it
// can appear in SO lines or Excel parses.
type UpsertPackingUnitInput struct {
	SKUID         uuid.UUID `json:"sku_id"`
	Unit          string    `json:"unit"`
	PiecesPerUnit int       `json:"pieces_per_unit"`
	IsDefault     bool      `json:"is_default"`
}

type BOMComponent struct {
	MaterialID      uuid.UUID    `json:"material_id"`
	MaterialType    MaterialType `json:"material_type"`
	QuantityPerUnit float64      `json:"quantity_per_unit"`
	Unit            string       `json:"unit"`
}

type BOM struct {
	SKUID      uuid.UUID      `json:"sku_id"`
	Components []BOMComponent `json:"components"`
}

type SetBOMInput struct {
	SKUID      uuid.UUID      `json:"sku_id"`
	Components []BOMComponent `json:"components"`
}

type BOMVariant struct {
	ID          uuid.UUID `json:"id"`
	SKUID       uuid.UUID `json:"sku_id"`
	VariantCode string    `json:"variant_code"`
	Name        string    `json:"name"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateBOMVariantInput struct {
	SKUID       uuid.UUID      `json:"sku_id"`
	VariantCode string         `json:"variant_code"`
	Name        string         `json:"name"`
	Components  []BOMComponent `json:"components"`
}

type Service interface {
	CreateMaterial(ctx context.Context, in CreateMaterialInput) (Material, error)
	ListMaterials(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[Material], error)
	GetMaterial(ctx context.Context, materialID uuid.UUID) (Material, error)
	DeactivateMaterial(ctx context.Context, materialID uuid.UUID) error

	// UpdateMinRemnantPolicy adjusts the per-material thresholds used by
	// inventory.RecordCut to drop sub-threshold remnants into waste
	// (BR-K06/K07/K08). Both values must be non-negative; 0 disables the
	// corresponding axis. Audit is logged by the catalog service.
	UpdateMinRemnantPolicy(ctx context.Context, in UpdateMinRemnantPolicyInput) (Material, error)

	CreateSKU(ctx context.Context, in CreateSKUInput) (SKU, error)
	ListSKUs(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[SKU], error)
	GetSKU(ctx context.Context, skuID uuid.UUID) (SKU, error)
	DeactivateSKU(ctx context.Context, skuID uuid.UUID) error
	// UpdateSKU sets export/shipping fields (BR-SKU02 validates hs_code format).
	UpdateSKU(ctx context.Context, in UpdateSKUInput) (SKU, error)
	// ExportSKUs writes up to limit SKUs as an .xlsx workbook to w.
	ExportSKUs(ctx context.Context, p httpkit.PageParams, w io.Writer) error

	// UpsertSKUComponent creates or replaces one (sku_id, component_type) row.
	// BR-PK-MULTI01: component_type must be a non-empty string unique per SKU.
	UpsertSKUComponent(ctx context.Context, in UpsertSKUComponentInput) (SKUComponent, error)
	ListSKUComponents(ctx context.Context, skuID uuid.UUID) ([]SKUComponent, error)
	DeleteSKUComponent(ctx context.Context, skuID uuid.UUID, componentType string) error

	// UpsertPackingUnit creates or replaces a (sku_id, unit) row.
	// Setting is_default=true automatically clears is_default on all other units
	// for that SKU so the invariant "exactly one default" is maintained.
	UpsertPackingUnit(ctx context.Context, in UpsertPackingUnitInput) (PackingUnit, error)
	ListPackingUnits(ctx context.Context, skuID uuid.UUID) ([]PackingUnit, error)
	DeletePackingUnit(ctx context.Context, skuID uuid.UUID, unit string) error

	SetBOM(ctx context.Context, in SetBOMInput) (BOM, error)
	GetBOM(ctx context.Context, skuID uuid.UUID) (BOM, error)

	// CreateBOMVariant registers a named variant with its own component list.
	// variant_code must be unique per SKU and must not be "DEFAULT".
	CreateBOMVariant(ctx context.Context, in CreateBOMVariantInput) (BOMVariant, error)
	ListBOMVariants(ctx context.Context, skuID uuid.UUID) ([]BOMVariant, error)
	// GetBOMForVariant returns the BOM for the given variant code.
	// When variantCode is empty it falls back to the DEFAULT variant, then to
	// the legacy bom_components table if no DEFAULT variant exists.
	GetBOMForVariant(ctx context.Context, skuID uuid.UUID, variantCode string) (BOM, error)
}
