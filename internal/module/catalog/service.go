package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	st store
}

func NewService(st store) Service {
	return &service{st: st}
}

func validMaterialType(t MaterialType) bool {
	switch t {
	case MaterialTypePlywood, MaterialTypeGlue, MaterialTypeMetal, MaterialTypeOther:
		return true
	default:
		return false
	}
}

func (s *service) CreateMaterial(ctx context.Context, in CreateMaterialInput) (Material, error) {
	if in.Name == "" {
		return Material{}, domain.NewBizError(domain.ErrInvalidInput, "material name is required")
	}
	if !validMaterialType(in.Type) {
		return Material{}, domain.NewBizError(domain.ErrInvalidInput, "invalid material type")
	}
	if in.Unit == "" {
		return Material{}, domain.NewBizError(domain.ErrInvalidInput, "material unit is required")
	}

	m := Material{
		ID:        uuid.New(),
		Type:      in.Type,
		Name:      in.Name,
		Unit:      in.Unit,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.st.insertMaterial(ctx, m); err != nil {
		return Material{}, err
	}
	return m, nil
}

func (s *service) ListMaterials(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[Material], error) {
	items, total, err := s.st.selectMaterialsPaged(ctx, p)
	if err != nil {
		return httpkit.PagedResult[Material]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

func (s *service) GetMaterial(ctx context.Context, materialID uuid.UUID) (Material, error) {
	return s.st.selectMaterialByID(ctx, materialID)
}

func (s *service) DeactivateMaterial(ctx context.Context, materialID uuid.UUID) error {
	return s.st.deactivateMaterial(ctx, materialID)
}

func (s *service) CreateSKU(ctx context.Context, in CreateSKUInput) (SKU, error) {
	if in.Code == "" {
		return SKU{}, domain.NewBizError(domain.ErrInvalidInput, "SKU code is required")
	}
	if !in.Dimensions.Valid() {
		return SKU{}, domain.NewBizError(domain.ErrInvalidInput, "invalid dimensions")
	}

	sku := SKU{
		ID:            uuid.New(),
		Code:          in.Code,
		Name:          in.Name,
		Dimensions:    in.Dimensions,
		RequiresMetal: in.RequiresMetal,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.st.insertSKU(ctx, sku); err != nil {
		return SKU{}, err
	}
	return sku, nil
}

func (s *service) ListSKUs(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[SKU], error) {
	items, total, err := s.st.selectSKUsPaged(ctx, p)
	if err != nil {
		return httpkit.PagedResult[SKU]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

func (s *service) GetSKU(ctx context.Context, skuID uuid.UUID) (SKU, error) {
	return s.st.selectSKUByID(ctx, skuID)
}

func (s *service) DeactivateSKU(ctx context.Context, skuID uuid.UUID) error {
	return s.st.deactivateSKU(ctx, skuID)
}

func (s *service) SetBOM(ctx context.Context, in SetBOMInput) (BOM, error) {
	if _, err := s.st.selectSKUByID(ctx, in.SKUID); err != nil {
		return BOM{}, err
	}
	for i, c := range in.Components {
		if !validMaterialType(c.MaterialType) {
			return BOM{}, domain.NewBizError(domain.ErrInvalidInput, fmt.Sprintf("invalid material type in component %d", i+1))
		}
		if c.QuantityPerUnit <= 0 {
			return BOM{}, domain.NewBizError(domain.ErrInvalidInput, fmt.Sprintf("quantity_per_unit must be positive in component %d", i+1))
		}
		if _, err := s.st.selectMaterialByID(ctx, c.MaterialID); err != nil {
			return BOM{}, err
		}
	}
	if err := s.st.upsertBOM(ctx, in.SKUID, in.Components); err != nil {
		return BOM{}, err
	}
	return BOM(in), nil
}

func (s *service) GetBOM(ctx context.Context, skuID uuid.UUID) (BOM, error) {
	return s.st.selectBOMBySKU(ctx, skuID)
}
