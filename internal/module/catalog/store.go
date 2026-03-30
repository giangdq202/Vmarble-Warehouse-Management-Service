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

	insertSKU(ctx context.Context, s SKU) error
	selectSKUs(ctx context.Context) ([]SKU, error)
	selectSKUsPaged(ctx context.Context, p httpkit.PageParams) ([]SKU, int, error)
	selectSKUByID(ctx context.Context, id uuid.UUID) (SKU, error)

	upsertBOM(ctx context.Context, skuID uuid.UUID, components []BOMComponent) error
	selectBOMBySKU(ctx context.Context, skuID uuid.UUID) (BOM, error)
}
