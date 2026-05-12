package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mockStore ────────────────────────────────────────────────────────────────
// Minimal hand-written mock that satisfies the catalog store interface.

type mockStore struct {
	// insertMaterial
	insertMaterialErr error

	// selectMaterials (unbounded)
	selectMaterialsResult []Material
	selectMaterialsErr    error

	// selectMaterialsPaged
	selectMaterialsPagedItems []Material
	selectMaterialsPagedTotal int
	selectMaterialsPagedErr   error

	// selectMaterialByID
	selectMaterialByIDResult Material
	selectMaterialByIDErr    error

	// deactivateMaterial
	deactivateMaterialErr error

	// insertSKU
	insertSKUErr error

	// selectSKUs (unbounded)
	selectSKUsResult []SKU
	selectSKUsErr    error

	// selectSKUsPaged
	selectSKUsPagedItems []SKU
	selectSKUsPagedTotal int
	selectSKUsPagedErr   error

	// selectSKUByID
	selectSKUByIDResult SKU
	selectSKUByIDErr    error

	// deactivateSKU
	deactivateSKUErr error

	// upsertBOM
	upsertBOMErr error

	// selectBOMBySKU
	selectBOMBySKUResult BOM
	selectBOMBySKUErr    error

	// insertBOMVariant
	insertBOMVariantErr error

	// selectBOMVariantsBySkuID
	selectBOMVariantsBySkuIDResult []BOMVariant
	selectBOMVariantsBySkuIDErr    error

	// selectBOMVariantByCode
	selectBOMVariantByCodeResult BOMVariant
	selectBOMVariantByCodeErr    error

	// selectBOMComponentsByVariantID
	selectBOMComponentsByVariantIDResult []BOMComponent
	selectBOMComponentsByVariantIDErr    error
}

func (m *mockStore) insertMaterial(_ context.Context, _ Material) error {
	return m.insertMaterialErr
}
func (m *mockStore) selectMaterials(_ context.Context) ([]Material, error) {
	return m.selectMaterialsResult, m.selectMaterialsErr
}
func (m *mockStore) selectMaterialsPaged(_ context.Context, _ httpkit.PageParams) ([]Material, int, error) {
	return m.selectMaterialsPagedItems, m.selectMaterialsPagedTotal, m.selectMaterialsPagedErr
}
func (m *mockStore) selectMaterialByID(_ context.Context, _ uuid.UUID) (Material, error) {
	return m.selectMaterialByIDResult, m.selectMaterialByIDErr
}
func (m *mockStore) deactivateMaterial(_ context.Context, _ uuid.UUID) error {
	return m.deactivateMaterialErr
}
func (m *mockStore) insertSKU(_ context.Context, _ SKU) error {
	return m.insertSKUErr
}
func (m *mockStore) selectSKUs(_ context.Context) ([]SKU, error) {
	return m.selectSKUsResult, m.selectSKUsErr
}
func (m *mockStore) selectSKUsPaged(_ context.Context, _ httpkit.PageParams) ([]SKU, int, error) {
	return m.selectSKUsPagedItems, m.selectSKUsPagedTotal, m.selectSKUsPagedErr
}
func (m *mockStore) selectSKUByID(_ context.Context, _ uuid.UUID) (SKU, error) {
	return m.selectSKUByIDResult, m.selectSKUByIDErr
}
func (m *mockStore) deactivateSKU(_ context.Context, _ uuid.UUID) error {
	return m.deactivateSKUErr
}
func (m *mockStore) upsertBOM(_ context.Context, _ uuid.UUID, _ []BOMComponent) error {
	return m.upsertBOMErr
}
func (m *mockStore) selectBOMBySKU(_ context.Context, _ uuid.UUID) (BOM, error) {
	return m.selectBOMBySKUResult, m.selectBOMBySKUErr
}
func (m *mockStore) insertBOMVariant(_ context.Context, _ BOMVariant, _ []BOMComponent) error {
	return m.insertBOMVariantErr
}
func (m *mockStore) selectBOMVariantsBySkuID(_ context.Context, _ uuid.UUID) ([]BOMVariant, error) {
	return m.selectBOMVariantsBySkuIDResult, m.selectBOMVariantsBySkuIDErr
}
func (m *mockStore) selectBOMVariantByCode(_ context.Context, _ uuid.UUID, _ string) (BOMVariant, error) {
	return m.selectBOMVariantByCodeResult, m.selectBOMVariantByCodeErr
}
func (m *mockStore) selectBOMComponentsByVariantID(_ context.Context, _ uuid.UUID) ([]BOMComponent, error) {
	return m.selectBOMComponentsByVariantIDResult, m.selectBOMComponentsByVariantIDErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newMaterial(name string) Material {
	return Material{
		ID:        uuid.New(),
		Type:      MaterialTypePlywood,
		Name:      name,
		Unit:      "sheet",
		CreatedAt: time.Now().UTC(),
	}
}

func newSKU(code, name string) SKU {
	return SKU{
		ID:         uuid.New(),
		Code:       code,
		Name:       name,
		Dimensions: domain.Dimension{LengthMM: 1200, WidthMM: 600},
		CreatedAt:  time.Now().UTC(),
	}
}

// ── ListMaterials (paginated) ─────────────────────────────────────────────────

func TestListMaterials_ReturnsPagedResult(t *testing.T) {
	items := []Material{newMaterial("Oak"), newMaterial("Pine")}
	st := &mockStore{
		selectMaterialsPagedItems: items,
		selectMaterialsPagedTotal: 2,
	}
	svc := NewService(st)

	p := httpkit.PageParams{Page: 1, Limit: 10}
	result, err := svc.ListMaterials(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("items = %d, want 2", len(result.Items))
	}
	if result.TotalItems != 2 {
		t.Errorf("total_items = %d, want 2", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1", result.TotalPages)
	}
	if result.CurrentPage != 1 {
		t.Errorf("current_page = %d, want 1", result.CurrentPage)
	}
	if result.Limit != 10 {
		t.Errorf("limit = %d, want 10", result.Limit)
	}
}

func TestListMaterials_SearchNoResults_ReturnsEmptyItems(t *testing.T) {
	st := &mockStore{
		selectMaterialsPagedItems: nil,
		selectMaterialsPagedTotal: 0,
	}
	svc := NewService(st)

	p := httpkit.PageParams{Page: 1, Limit: 10, Search: "nonexistent-xyz"}
	result, err := svc.ListMaterials(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0 for no-match search", len(result.Items))
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if result.TotalPages != 1 {
		// Always at least 1 page even when empty
		t.Errorf("total_pages = %d, want 1 (minimum)", result.TotalPages)
	}
}

func TestListMaterials_LastPage_CorrectMetadata(t *testing.T) {
	// 25 total items, page size 10 → 3 pages; page 3 has 5 items
	lastPageItems := make([]Material, 5)
	for i := range lastPageItems {
		lastPageItems[i] = newMaterial("Mat")
	}
	st := &mockStore{
		selectMaterialsPagedItems: lastPageItems,
		selectMaterialsPagedTotal: 25,
	}
	svc := NewService(st)

	p := httpkit.PageParams{Page: 3, Limit: 10}
	result, err := svc.ListMaterials(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalItems != 25 {
		t.Errorf("total_items = %d, want 25", result.TotalItems)
	}
	if result.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", result.TotalPages)
	}
	if result.CurrentPage != 3 {
		t.Errorf("current_page = %d, want 3", result.CurrentPage)
	}
	if len(result.Items) != 5 {
		t.Errorf("items on last page = %d, want 5", len(result.Items))
	}
}

func TestListMaterials_StoreError_Propagated(t *testing.T) {
	storeErr := errors.New("database failure")
	st := &mockStore{selectMaterialsPagedErr: storeErr}
	svc := NewService(st)

	_, err := svc.ListMaterials(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("got %v, want %v", err, storeErr)
	}
}

// ── ListSKUs (paginated) ──────────────────────────────────────────────────────

func TestListSKUs_ReturnsPagedResult(t *testing.T) {
	items := []SKU{newSKU("SKU-A", "Panel A"), newSKU("SKU-B", "Panel B"), newSKU("SKU-C", "Panel C")}
	st := &mockStore{
		selectSKUsPagedItems: items,
		selectSKUsPagedTotal: 3,
	}
	svc := NewService(st)

	p := httpkit.PageParams{Page: 1, Limit: 10}
	result, err := svc.ListSKUs(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Errorf("items = %d, want 3", len(result.Items))
	}
	if result.TotalItems != 3 {
		t.Errorf("total_items = %d, want 3", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1", result.TotalPages)
	}
}

func TestListSKUs_SearchNoResults_ReturnsEmptyItems(t *testing.T) {
	st := &mockStore{
		selectSKUsPagedItems: nil,
		selectSKUsPagedTotal: 0,
	}
	svc := NewService(st)

	p := httpkit.PageParams{Page: 1, Limit: 10, Search: "SKU-DOES-NOT-EXIST"}
	result, err := svc.ListSKUs(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0 for no-match search", len(result.Items))
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1 (minimum)", result.TotalPages)
	}
}

func TestListSKUs_LastPage_CorrectMetadata(t *testing.T) {
	// 11 total, limit 5 → 3 pages; last page has 1 item
	lastPageItems := []SKU{newSKU("SKU-LAST", "Last Panel")}
	st := &mockStore{
		selectSKUsPagedItems: lastPageItems,
		selectSKUsPagedTotal: 11,
	}
	svc := NewService(st)

	p := httpkit.PageParams{Page: 3, Limit: 5}
	result, err := svc.ListSKUs(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalItems != 11 {
		t.Errorf("total_items = %d, want 11", result.TotalItems)
	}
	if result.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", result.TotalPages)
	}
	if result.CurrentPage != 3 {
		t.Errorf("current_page = %d, want 3", result.CurrentPage)
	}
	if len(result.Items) != 1 {
		t.Errorf("items on last page = %d, want 1", len(result.Items))
	}
}

func TestListSKUs_StoreError_Propagated(t *testing.T) {
	storeErr := errors.New("connection reset")
	st := &mockStore{selectSKUsPagedErr: storeErr}
	svc := NewService(st)

	_, err := svc.ListSKUs(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("got %v, want %v", err, storeErr)
	}
}

// ── httpkit.PageParams helpers ────────────────────────────────────────────────

func TestPageParams_Offset(t *testing.T) {
	tests := []struct {
		page, limit, wantOffset int
	}{
		{1, 10, 0},
		{2, 10, 10},
		{3, 10, 20},
		{1, 25, 0},
		{2, 25, 25},
	}
	for _, tc := range tests {
		p := httpkit.PageParams{Page: tc.page, Limit: tc.limit}
		if got := p.Offset(); got != tc.wantOffset {
			t.Errorf("PageParams{Page:%d, Limit:%d}.Offset() = %d, want %d",
				tc.page, tc.limit, got, tc.wantOffset)
		}
	}
}

func TestNewPagedResult_EmptySlice_NotNilItems(t *testing.T) {
	// Verify nil slice is normalised to empty slice so JSON serialises as [] not null
	p := httpkit.PageParams{Page: 1, Limit: 10}
	result := httpkit.NewPagedResult[Material](nil, 0, p)
	if result.Items == nil {
		t.Error("Items must not be nil (should be empty slice for JSON [])")
	}
}

func TestNewPagedResult_TotalPagesMinimumOne(t *testing.T) {
	p := httpkit.PageParams{Page: 1, Limit: 10}
	result := httpkit.NewPagedResult[Material](nil, 0, p)
	if result.TotalPages < 1 {
		t.Errorf("total_pages = %d, want at least 1", result.TotalPages)
	}
}

// ── CreateMaterial / CreateSKU (existing behaviour still works) ───────────────

func TestCreateMaterial_MissingName_Rejected(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.CreateMaterial(context.Background(), CreateMaterialInput{
		Type: MaterialTypePlywood,
		Unit: "sheet",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing name, got %v", err)
	}
}

func TestCreateSKU_InvalidDimensions_Rejected(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.CreateSKU(context.Background(), CreateSKUInput{
		Code:       "SKU-X",
		Dimensions: domain.Dimension{LengthMM: 0, WidthMM: -1},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero/negative dimensions, got %v", err)
	}
}

// ── CreateBOMVariant ──────────────────────────────────────────────────────────

func TestCreateBOMVariant_HappyPath_ReturnsVariant(t *testing.T) {
	skuID := uuid.New()
	matID := uuid.New()
	st := &mockStore{
		selectSKUByIDResult: newSKU("SKU-A", "Panel A"),
		selectMaterialByIDResult: Material{
			ID: matID, Type: MaterialTypePlywood, Name: "Oak", Unit: "sheet", IsActive: true,
		},
	}
	svc := NewService(st)

	in := CreateBOMVariantInput{
		SKUID:       skuID,
		VariantCode: "VARIANT-1",
		Name:        "Variant One",
		Components: []BOMComponent{
			{MaterialID: matID, MaterialType: MaterialTypePlywood, QuantityPerUnit: 2, Unit: "sheet"},
		},
	}
	v, err := svc.CreateBOMVariant(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.VariantCode != "VARIANT-1" {
		t.Errorf("variant_code = %q, want VARIANT-1", v.VariantCode)
	}
	if v.IsDefault {
		t.Error("user-created variant must not be default")
	}
}

func TestCreateBOMVariant_ReservedDefaultCode_Rejected(t *testing.T) {
	svc := NewService(&mockStore{selectSKUByIDResult: newSKU("SKU-A", "Panel A")})
	_, err := svc.CreateBOMVariant(context.Background(), CreateBOMVariantInput{
		SKUID:       uuid.New(),
		VariantCode: "DEFAULT",
		Name:        "Default",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for reserved code, got %v", err)
	}
}

func TestCreateBOMVariant_EmptyCode_Rejected(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.CreateBOMVariant(context.Background(), CreateBOMVariantInput{
		SKUID: uuid.New(),
		Name:  "Some Variant",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty code, got %v", err)
	}
}

func TestCreateBOMVariant_UnknownSKU_ReturnsNotFound(t *testing.T) {
	st := &mockStore{selectSKUByIDErr: domain.ErrNotFound}
	svc := NewService(st)
	_, err := svc.CreateBOMVariant(context.Background(), CreateBOMVariantInput{
		SKUID:       uuid.New(),
		VariantCode: "V1",
		Name:        "V1",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown SKU, got %v", err)
	}
}

func TestCreateBOMVariant_NegativeQty_Rejected(t *testing.T) {
	st := &mockStore{selectSKUByIDResult: newSKU("SKU-A", "Panel A")}
	svc := NewService(st)
	_, err := svc.CreateBOMVariant(context.Background(), CreateBOMVariantInput{
		SKUID:       uuid.New(),
		VariantCode: "V1",
		Name:        "V1",
		Components: []BOMComponent{
			{MaterialID: uuid.New(), MaterialType: MaterialTypePlywood, QuantityPerUnit: -1, Unit: "sheet"},
		},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative qty, got %v", err)
	}
}

// ── GetBOMForVariant ──────────────────────────────────────────────────────────

func TestGetBOMForVariant_ExplicitCode_ReturnsVariantComponents(t *testing.T) {
	skuID := uuid.New()
	variantID := uuid.New()
	matID := uuid.New()
	st := &mockStore{
		selectSKUByIDResult: newSKU("SKU-A", "Panel A"),
		selectBOMVariantByCodeResult: BOMVariant{
			ID: variantID, SKUID: skuID, VariantCode: "VARIANT-1", Name: "V1",
		},
		selectBOMComponentsByVariantIDResult: []BOMComponent{
			{MaterialID: matID, MaterialType: MaterialTypePlywood, QuantityPerUnit: 3, Unit: "sheet"},
		},
	}
	svc := NewService(st)

	bom, err := svc.GetBOMForVariant(context.Background(), skuID, "VARIANT-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bom.Components) != 1 {
		t.Errorf("components = %d, want 1", len(bom.Components))
	}
	if bom.Components[0].QuantityPerUnit != 3 {
		t.Errorf("qty = %v, want 3", bom.Components[0].QuantityPerUnit)
	}
}

func TestGetBOMForVariant_EmptyCode_FallsBackToDefault(t *testing.T) {
	skuID := uuid.New()
	variantID := uuid.New()
	matID := uuid.New()
	st := &mockStore{
		selectSKUByIDResult: newSKU("SKU-A", "Panel A"),
		selectBOMVariantByCodeResult: BOMVariant{
			ID: variantID, SKUID: skuID, VariantCode: "DEFAULT", Name: "Default", IsDefault: true,
		},
		selectBOMComponentsByVariantIDResult: []BOMComponent{
			{MaterialID: matID, MaterialType: MaterialTypePlywood, QuantityPerUnit: 1, Unit: "sheet"},
		},
	}
	svc := NewService(st)

	bom, err := svc.GetBOMForVariant(context.Background(), skuID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bom.Components) != 1 {
		t.Errorf("components = %d, want 1", len(bom.Components))
	}
}

func TestGetBOMForVariant_EmptyCode_NoDefaultVariant_FallsBackToLegacy(t *testing.T) {
	skuID := uuid.New()
	matID := uuid.New()
	st := &mockStore{
		selectSKUByIDResult:       newSKU("SKU-A", "Panel A"),
		selectBOMVariantByCodeErr: domain.ErrNotFound,
		selectBOMBySKUResult: BOM{
			SKUID: skuID,
			Components: []BOMComponent{
				{MaterialID: matID, MaterialType: MaterialTypePlywood, QuantityPerUnit: 2, Unit: "sheet"},
			},
		},
	}
	svc := NewService(st)

	bom, err := svc.GetBOMForVariant(context.Background(), skuID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bom.Components) != 1 {
		t.Errorf("components = %d, want 1 (legacy fallback)", len(bom.Components))
	}
}

func TestGetBOMForVariant_UnknownExplicitCode_ReturnsNotFound(t *testing.T) {
	st := &mockStore{
		selectSKUByIDResult:       newSKU("SKU-A", "Panel A"),
		selectBOMVariantByCodeErr: domain.ErrNotFound,
	}
	svc := NewService(st)

	_, err := svc.GetBOMForVariant(context.Background(), uuid.New(), "NONEXISTENT")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown variant code, got %v", err)
	}
}

// ── ListBOMVariants ───────────────────────────────────────────────────────────

func TestListBOMVariants_ReturnsList(t *testing.T) {
	skuID := uuid.New()
	st := &mockStore{
		selectSKUByIDResult: newSKU("SKU-A", "Panel A"),
		selectBOMVariantsBySkuIDResult: []BOMVariant{
			{ID: uuid.New(), SKUID: skuID, VariantCode: "DEFAULT", Name: "Default", IsDefault: true},
			{ID: uuid.New(), SKUID: skuID, VariantCode: "V1", Name: "Variant 1"},
		},
	}
	svc := NewService(st)

	variants, err := svc.ListBOMVariants(context.Background(), skuID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(variants) != 2 {
		t.Errorf("variants = %d, want 2", len(variants))
	}
}

func TestListBOMVariants_NoVariants_ReturnsEmptySlice(t *testing.T) {
	st := &mockStore{
		selectSKUByIDResult:            newSKU("SKU-A", "Panel A"),
		selectBOMVariantsBySkuIDResult: nil,
	}
	svc := NewService(st)

	variants, err := svc.ListBOMVariants(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if variants == nil {
		t.Error("variants must not be nil (should be empty slice for JSON [])")
	}
	if len(variants) != 0 {
		t.Errorf("variants = %d, want 0", len(variants))
	}
}
