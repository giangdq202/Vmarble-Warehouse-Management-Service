//go:build integration

package catalog

// pgstore_integration_test.go — integration tests for the catalog pgstore.
//
// Tests run against a real PostgreSQL 17 container (managed by testcontainers-go).
// They verify correct persistence, retrieval, BOM round-trips, and ErrNotFound
// behaviour for the catalog module's pgstore.
//
// Run with:
//
//	make test-integration
//	# or directly:
//	go test -tags integration ./internal/module/catalog/... -v -count=1

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
	"github.com/vmarble/warehouse-management-service/internal/testhelper"
)

// ── shared pool ──────────────────────────────────────────────────────────────

var (
	sharedPool *pgxpool.Pool
	setupOnce  sync.Once
)

func getPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	setupOnce.Do(func() {
		sharedPool = testhelper.StartTestDB(t)
	})
	return sharedPool
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// ── helpers ───────────────────────────────────────────────────────────────────

func truncateCatalog(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{"bom_components", "skus", "materials"} {
		if _, err := sharedPool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}

func newSvc(pool *pgxpool.Pool) Service {
	return NewService(NewPGStore(pool))
}

// ── Material ─────────────────────────────────────────────────────────────────

func TestIntegration_CreateMaterial_HappyPath(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	m, err := svc.CreateMaterial(context.Background(), CreateMaterialInput{
		Type: MaterialTypePlywood,
		Name: "Oak Plywood 18mm",
		Unit: "sheet",
	})
	if err != nil {
		t.Fatalf("CreateMaterial: %v", err)
	}
	if m.ID == uuid.Nil {
		t.Error("material.ID must be set")
	}
	if m.Type != MaterialTypePlywood {
		t.Errorf("material.Type = %v, want PLYWOOD", m.Type)
	}
	if m.Name != "Oak Plywood 18mm" {
		t.Errorf("material.Name = %q, want 'Oak Plywood 18mm'", m.Name)
	}
}

func TestIntegration_ListMaterials_ReturnsPersisted(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	for _, name := range []string{"A", "B", "C"} {
		_, err := svc.CreateMaterial(context.Background(), CreateMaterialInput{
			Type: MaterialTypeGlue,
			Name: name,
			Unit: "kg",
		})
		if err != nil {
			t.Fatalf("CreateMaterial(%s): %v", name, err)
		}
	}

	result, err := svc.ListMaterials(context.Background(), httpkit.PageParams{Page: 1, Limit: 100})
	if err != nil {
		t.Fatalf("ListMaterials: %v", err)
	}
	if len(result.Items) != 3 {
		t.Errorf("materials count = %d, want 3", len(result.Items))
	}
}

func TestIntegration_GetMaterial_NotFound(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	_, err := svc.GetMaterial(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_GetMaterial_RoundTrip(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	created, err := svc.CreateMaterial(context.Background(), CreateMaterialInput{
		Type: MaterialTypeMetal,
		Name: "Steel Sheet",
		Unit: "kg",
	})
	if err != nil {
		t.Fatalf("CreateMaterial: %v", err)
	}

	fetched, err := svc.GetMaterial(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetMaterial: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: got %v, want %v", fetched.ID, created.ID)
	}
	if fetched.Name != created.Name {
		t.Errorf("Name mismatch: got %q, want %q", fetched.Name, created.Name)
	}
}

func TestIntegration_CreateMaterial_SetsCreatedAt(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	before := time.Now().UTC().Add(-time.Second)
	m, _ := svc.CreateMaterial(context.Background(), CreateMaterialInput{
		Type: MaterialTypeOther, Name: "Misc", Unit: "pcs",
	})
	after := time.Now().UTC().Add(time.Second)

	if m.CreatedAt.Before(before) || m.CreatedAt.After(after) {
		t.Errorf("material.CreatedAt = %v, want between %v and %v", m.CreatedAt, before, after)
	}
}

// ── SKU ───────────────────────────────────────────────────────────────────────

func TestIntegration_CreateSKU_HappyPath(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	sku, err := svc.CreateSKU(context.Background(), CreateSKUInput{
		Code:          "SKU-INT-001",
		Name:          "Integration Panel",
		Dimensions:    domain.Dimension{LengthMM: 1200, WidthMM: 600},
		RequiresMetal: false,
	})
	if err != nil {
		t.Fatalf("CreateSKU: %v", err)
	}
	if sku.ID == uuid.Nil {
		t.Error("sku.ID must be set")
	}
	if sku.Code != "SKU-INT-001" {
		t.Errorf("sku.Code = %q, want SKU-INT-001", sku.Code)
	}
}

func TestIntegration_CreateSKU_DuplicateCode_Rejected(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	in := CreateSKUInput{
		Code:       "SKU-DUP",
		Name:       "Dup Panel",
		Dimensions: domain.Dimension{LengthMM: 1000, WidthMM: 500},
	}
	if _, err := svc.CreateSKU(context.Background(), in); err != nil {
		t.Fatalf("first CreateSKU: %v", err)
	}
	_, err := svc.CreateSKU(context.Background(), in)
	if err == nil {
		t.Error("expected error on duplicate SKU code, got nil")
	}
}

func TestIntegration_GetSKU_NotFound(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	_, err := svc.GetSKU(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_ListSKUs_ReturnsPersisted(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	for i := range 4 {
		_, err := svc.CreateSKU(context.Background(), CreateSKUInput{
			Code:       fmt.Sprintf("SKU-LIST-%02d", i),
			Name:       fmt.Sprintf("Panel %d", i),
			Dimensions: domain.Dimension{LengthMM: 1000, WidthMM: 500},
		})
		if err != nil {
			t.Fatalf("CreateSKU[%d]: %v", i, err)
		}
	}

	result, err := svc.ListSKUs(context.Background(), httpkit.PageParams{Page: 1, Limit: 100})
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(result.Items) != 4 {
		t.Errorf("skus count = %d, want 4", len(result.Items))
	}
}

// ── BOM ───────────────────────────────────────────────────────────────────────

func TestIntegration_SetBOM_GetBOM_RoundTrip(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	mat, _ := svc.CreateMaterial(context.Background(), CreateMaterialInput{
		Type: MaterialTypePlywood, Name: "Plywood A", Unit: "sheet",
	})
	sku, _ := svc.CreateSKU(context.Background(), CreateSKUInput{
		Code:       "SKU-BOM-001",
		Name:       "BOM Panel",
		Dimensions: domain.Dimension{LengthMM: 1200, WidthMM: 600},
	})

	bom, err := svc.SetBOM(context.Background(), SetBOMInput{
		SKUID: sku.ID,
		Components: []BOMComponent{
			{
				MaterialID:      mat.ID,
				MaterialType:    MaterialTypePlywood,
				QuantityPerUnit: 1.5,
				Unit:            "sheet",
			},
		},
	})
	if err != nil {
		t.Fatalf("SetBOM: %v", err)
	}
	if len(bom.Components) != 1 {
		t.Fatalf("bom.Components = %d, want 1", len(bom.Components))
	}

	fetched, err := svc.GetBOM(context.Background(), sku.ID)
	if err != nil {
		t.Fatalf("GetBOM: %v", err)
	}
	if len(fetched.Components) != 1 {
		t.Fatalf("fetched.Components = %d, want 1", len(fetched.Components))
	}
	c := fetched.Components[0]
	if c.MaterialID != mat.ID {
		t.Errorf("component.MaterialID = %v, want %v", c.MaterialID, mat.ID)
	}
	if c.QuantityPerUnit != 1.5 {
		t.Errorf("component.QuantityPerUnit = %v, want 1.5", c.QuantityPerUnit)
	}
}

func TestIntegration_SetBOM_Replaces_ExistingComponents(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	mat1, _ := svc.CreateMaterial(context.Background(), CreateMaterialInput{
		Type: MaterialTypePlywood, Name: "Mat1", Unit: "sheet",
	})
	mat2, _ := svc.CreateMaterial(context.Background(), CreateMaterialInput{
		Type: MaterialTypeGlue, Name: "Mat2", Unit: "kg",
	})
	sku, _ := svc.CreateSKU(context.Background(), CreateSKUInput{
		Code:       "SKU-BOM-REPLACE",
		Name:       "Replace Panel",
		Dimensions: domain.Dimension{LengthMM: 1000, WidthMM: 500},
	})

	// First BOM: 2 components.
	svc.SetBOM(context.Background(), SetBOMInput{
		SKUID: sku.ID,
		Components: []BOMComponent{
			{MaterialID: mat1.ID, MaterialType: MaterialTypePlywood, QuantityPerUnit: 1, Unit: "sheet"},
			{MaterialID: mat2.ID, MaterialType: MaterialTypeGlue, QuantityPerUnit: 0.5, Unit: "kg"},
		},
	})

	// Second BOM: 1 component — must replace, not append.
	bom, err := svc.SetBOM(context.Background(), SetBOMInput{
		SKUID: sku.ID,
		Components: []BOMComponent{
			{MaterialID: mat1.ID, MaterialType: MaterialTypePlywood, QuantityPerUnit: 2, Unit: "sheet"},
		},
	})
	if err != nil {
		t.Fatalf("SetBOM (replace): %v", err)
	}
	if len(bom.Components) != 1 {
		t.Errorf("after replace, bom.Components = %d, want 1", len(bom.Components))
	}

	fetched, _ := svc.GetBOM(context.Background(), sku.ID)
	if len(fetched.Components) != 1 {
		t.Errorf("after replace, fetched.Components = %d, want 1", len(fetched.Components))
	}
}

func TestIntegration_GetBOM_SKUNotFound(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	_, err := svc.GetBOM(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown SKU, got %v", err)
	}
}

// ── ListMaterials pagination & search ─────────────────────────────────────────

func TestIntegration_ListMaterials_Pagination_CorrectMetadata(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	// Seed 5 materials.
	for i := range 5 {
		_, err := svc.CreateMaterial(context.Background(), CreateMaterialInput{
			Type: MaterialTypePlywood,
			Name: fmt.Sprintf("PagMat-%02d", i),
			Unit: "sheet",
		})
		if err != nil {
			t.Fatalf("CreateMaterial[%d]: %v", i, err)
		}
	}

	// Page 1, limit 2 → 3 pages total.
	p1, err := svc.ListMaterials(context.Background(), httpkit.PageParams{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ListMaterials page 1: %v", err)
	}
	if p1.TotalItems != 5 {
		t.Errorf("total_items = %d, want 5", p1.TotalItems)
	}
	if p1.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", p1.TotalPages)
	}
	if p1.CurrentPage != 1 {
		t.Errorf("current_page = %d, want 1", p1.CurrentPage)
	}
	if len(p1.Items) != 2 {
		t.Errorf("page-1 items = %d, want 2", len(p1.Items))
	}

	// Last page (3) should have 1 item.
	p3, err := svc.ListMaterials(context.Background(), httpkit.PageParams{Page: 3, Limit: 2})
	if err != nil {
		t.Fatalf("ListMaterials page 3: %v", err)
	}
	if p3.TotalPages != 3 {
		t.Errorf("last page: total_pages = %d, want 3", p3.TotalPages)
	}
	if len(p3.Items) != 1 {
		t.Errorf("last-page items = %d, want 1", len(p3.Items))
	}
}

func TestIntegration_ListMaterials_Search_MatchesSubstring(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	svc.CreateMaterial(context.Background(), CreateMaterialInput{Type: MaterialTypePlywood, Name: "Oak Plywood", Unit: "sheet"})
	svc.CreateMaterial(context.Background(), CreateMaterialInput{Type: MaterialTypeGlue, Name: "Wood Glue", Unit: "kg"})
	svc.CreateMaterial(context.Background(), CreateMaterialInput{Type: MaterialTypeMetal, Name: "Steel Bar", Unit: "kg"})

	result, err := svc.ListMaterials(context.Background(), httpkit.PageParams{Page: 1, Limit: 10, Search: "plywood"})
	if err != nil {
		t.Fatalf("ListMaterials search: %v", err)
	}
	if result.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1 (ILIKE 'plywood')", result.TotalItems)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(result.Items))
	}
	if result.Items[0].Name != "Oak Plywood" {
		t.Errorf("matched material = %q, want 'Oak Plywood'", result.Items[0].Name)
	}
}

func TestIntegration_ListMaterials_Search_NoResults(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	svc.CreateMaterial(context.Background(), CreateMaterialInput{Type: MaterialTypePlywood, Name: "Cedar", Unit: "sheet"})

	result, err := svc.ListMaterials(context.Background(), httpkit.PageParams{Page: 1, Limit: 10, Search: "absolutely-not-there"})
	if err != nil {
		t.Fatalf("ListMaterials no-match search: %v", err)
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0", len(result.Items))
	}
	if result.TotalPages < 1 {
		t.Errorf("total_pages = %d, want at least 1", result.TotalPages)
	}
}

// ── ListSKUs pagination & search ──────────────────────────────────────────────

func TestIntegration_ListSKUs_Pagination_CorrectMetadata(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	for i := range 7 {
		_, err := svc.CreateSKU(context.Background(), CreateSKUInput{
			Code:       fmt.Sprintf("PAG-SKU-%02d", i),
			Name:       fmt.Sprintf("Paged Panel %02d", i),
			Dimensions: domain.Dimension{LengthMM: 1000, WidthMM: 500},
		})
		if err != nil {
			t.Fatalf("CreateSKU[%d]: %v", i, err)
		}
	}

	// Page 2, limit 3 → 3 pages total; page 2 has 3 items.
	p2, err := svc.ListSKUs(context.Background(), httpkit.PageParams{Page: 2, Limit: 3})
	if err != nil {
		t.Fatalf("ListSKUs page 2: %v", err)
	}
	if p2.TotalItems != 7 {
		t.Errorf("total_items = %d, want 7", p2.TotalItems)
	}
	if p2.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", p2.TotalPages)
	}
	if p2.CurrentPage != 2 {
		t.Errorf("current_page = %d, want 2", p2.CurrentPage)
	}
	if len(p2.Items) != 3 {
		t.Errorf("page-2 items = %d, want 3", len(p2.Items))
	}

	// Last page (3) has 1 item.
	p3, err := svc.ListSKUs(context.Background(), httpkit.PageParams{Page: 3, Limit: 3})
	if err != nil {
		t.Fatalf("ListSKUs page 3: %v", err)
	}
	if len(p3.Items) != 1 {
		t.Errorf("last-page items = %d, want 1", len(p3.Items))
	}
}

func TestIntegration_ListSKUs_SearchByCode(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	svc.CreateSKU(context.Background(), CreateSKUInput{Code: "CHAIR-001", Name: "Side Chair", Dimensions: domain.Dimension{LengthMM: 500, WidthMM: 500}})
	svc.CreateSKU(context.Background(), CreateSKUInput{Code: "TABLE-001", Name: "Dining Table", Dimensions: domain.Dimension{LengthMM: 1500, WidthMM: 900}})
	svc.CreateSKU(context.Background(), CreateSKUInput{Code: "SHELF-001", Name: "Wall Shelf", Dimensions: domain.Dimension{LengthMM: 800, WidthMM: 300}})

	result, err := svc.ListSKUs(context.Background(), httpkit.PageParams{Page: 1, Limit: 10, Search: "chair"})
	if err != nil {
		t.Fatalf("ListSKUs search by code: %v", err)
	}
	if result.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1 (ILIKE 'chair' on code or name)", result.TotalItems)
	}
}

func TestIntegration_ListSKUs_SearchByName(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	svc.CreateSKU(context.Background(), CreateSKUInput{Code: "ITEM-A", Name: "Wardrobe Panel", Dimensions: domain.Dimension{LengthMM: 2000, WidthMM: 600}})
	svc.CreateSKU(context.Background(), CreateSKUInput{Code: "ITEM-B", Name: "Bed Frame", Dimensions: domain.Dimension{LengthMM: 2100, WidthMM: 1600}})

	result, err := svc.ListSKUs(context.Background(), httpkit.PageParams{Page: 1, Limit: 10, Search: "wardrobe"})
	if err != nil {
		t.Fatalf("ListSKUs search by name: %v", err)
	}
	if result.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1 (ILIKE 'wardrobe' on name)", result.TotalItems)
	}
	if result.Items[0].Name != "Wardrobe Panel" {
		t.Errorf("matched sku name = %q, want 'Wardrobe Panel'", result.Items[0].Name)
	}
}

func TestIntegration_ListSKUs_Search_NoResults(t *testing.T) {
	pool := getPool(t)
	truncateCatalog(t)
	svc := newSvc(pool)

	svc.CreateSKU(context.Background(), CreateSKUInput{Code: "EXISTS", Name: "Existing Panel", Dimensions: domain.Dimension{LengthMM: 1200, WidthMM: 600}})

	result, err := svc.ListSKUs(context.Background(), httpkit.PageParams{Page: 1, Limit: 10, Search: "ZZZZZ-NO-MATCH"})
	if err != nil {
		t.Fatalf("ListSKUs no-match search: %v", err)
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0", len(result.Items))
	}
	if result.TotalPages < 1 {
		t.Errorf("total_pages = %d, want at least 1", result.TotalPages)
	}
}
