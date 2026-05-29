package catalog

import (
	"context"
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
	CreatedAt     time.Time        `json:"created_at"`
}

type CreateSKUInput struct {
	Code          string           `json:"code"`
	Name          string           `json:"name"`
	Dimensions    domain.Dimension `json:"dimensions"`
	RequiresMetal bool             `json:"requires_metal"`
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
