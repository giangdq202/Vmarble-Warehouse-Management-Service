package catalog

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertMaterial(ctx context.Context, m Material) error
	selectMaterials(ctx context.Context) ([]Material, error)
	selectMaterialsPaged(ctx context.Context, p httpkit.PageParams) ([]Material, int, error)
	selectMaterialByID(ctx context.Context, id uuid.UUID) (Material, error)
	deactivateMaterial(ctx context.Context, id uuid.UUID) error

	insertSKU(ctx context.Context, s SKU) error
	selectSKUs(ctx context.Context) ([]SKU, error)
	selectSKUsPaged(ctx context.Context, p httpkit.PageParams) ([]SKU, int, error)
	selectSKUByID(ctx context.Context, id uuid.UUID) (SKU, error)
	deactivateSKU(ctx context.Context, id uuid.UUID) error

	upsertBOM(ctx context.Context, skuID uuid.UUID, components []BOMComponent) error
	selectBOMBySKU(ctx context.Context, skuID uuid.UUID) (BOM, error)

	insertBOMVariant(ctx context.Context, v BOMVariant, components []BOMComponent) error
	selectBOMVariantsBySkuID(ctx context.Context, skuID uuid.UUID) ([]BOMVariant, error)
	selectBOMVariantByCode(ctx context.Context, skuID uuid.UUID, variantCode string) (BOMVariant, error)
	selectBOMComponentsByVariantID(ctx context.Context, variantID uuid.UUID) ([]BOMComponent, error)
}
