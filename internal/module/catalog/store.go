package catalog

import (
	"context"

	"github.com/google/uuid"
)

type store interface {
	insertMaterial(ctx context.Context, m Material) error
	selectMaterials(ctx context.Context) ([]Material, error)
	selectMaterialByID(ctx context.Context, id uuid.UUID) (Material, error)

	insertSKU(ctx context.Context, s SKU) error
	selectSKUs(ctx context.Context) ([]SKU, error)
	selectSKUByID(ctx context.Context, id uuid.UUID) (SKU, error)

	upsertBOM(ctx context.Context, skuID uuid.UUID, components []BOMComponent) error
	selectBOMBySKU(ctx context.Context, skuID uuid.UUID) (BOM, error)
}
