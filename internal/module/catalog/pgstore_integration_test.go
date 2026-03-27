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

	materials, err := svc.ListMaterials(context.Background())
	if err != nil {
		t.Fatalf("ListMaterials: %v", err)
	}
	if len(materials) != 3 {
		t.Errorf("materials count = %d, want 3", len(materials))
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

	skus, err := svc.ListSKUs(context.Background())
	if err != nil {
		t.Fatalf("ListSKUs: %v", err)
	}
	if len(skus) != 4 {
		t.Errorf("skus count = %d, want 4", len(skus))
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
