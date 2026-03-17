package catalog

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type MaterialType string

const (
	MaterialTypePlywood MaterialType = "PLYWOOD"
	MaterialTypeGlue    MaterialType = "GLUE"
	MaterialTypeMetal   MaterialType = "METAL"
	MaterialTypeOther   MaterialType = "OTHER"
)

type Material struct {
	ID        uuid.UUID    `json:"id"`
	Type      MaterialType `json:"type"`
	Name      string       `json:"name"`
	Unit      string       `json:"unit"`
	CreatedAt string       `json:"created_at"`
}

type CreateMaterialInput struct {
	Type MaterialType `json:"type"`
	Name string      `json:"name"`
	Unit string      `json:"unit"`
}

type SKU struct {
	ID           uuid.UUID       `json:"id"`
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Dimensions   domain.Dimension `json:"dimensions"`
	RequiresMetal bool           `json:"requires_metal"`
	CreatedAt    string          `json:"created_at"`
}

type CreateSKUInput struct {
	Code          string            `json:"code"`
	Name          string            `json:"name"`
	Dimensions    domain.Dimension   `json:"dimensions"`
	RequiresMetal bool              `json:"requires_metal"`
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

type Service interface {
	CreateMaterial(ctx context.Context, in CreateMaterialInput) (Material, error)
	ListMaterials(ctx context.Context) ([]Material, error)
	GetMaterial(ctx context.Context, materialID uuid.UUID) (Material, error)

	CreateSKU(ctx context.Context, in CreateSKUInput) (SKU, error)
	ListSKUs(ctx context.Context) ([]SKU, error)
	GetSKU(ctx context.Context, skuID uuid.UUID) (SKU, error)

	SetBOM(ctx context.Context, in SetBOMInput) (BOM, error)
	GetBOM(ctx context.Context, skuID uuid.UUID) (BOM, error)
}
