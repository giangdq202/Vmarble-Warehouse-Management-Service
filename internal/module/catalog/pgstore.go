package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) store {
	return &pgStore{pool: pool}
}

func (s *pgStore) insertMaterial(ctx context.Context, m Material) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO materials (id, type, name, unit, is_active, min_remnant_length_mm, min_remnant_width_mm, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		m.ID, m.Type, m.Name, m.Unit, m.IsActive, m.MinRemnantLengthMM, m.MinRemnantWidthMM, m.CreatedAt,
	)
	return err
}

func (s *pgStore) selectMaterials(ctx context.Context) ([]Material, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, type, name, unit, is_active, min_remnant_length_mm, min_remnant_width_mm, created_at
		 FROM materials WHERE is_active = true ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Material
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.Type, &m.Name, &m.Unit, &m.IsActive,
			&m.MinRemnantLengthMM, &m.MinRemnantWidthMM, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// selectMaterialsPaged returns a page of active materials optionally filtered by a
// case-insensitive keyword match on the name column.
// It returns (items, totalMatchingItems, error).
func (s *pgStore) selectMaterialsPaged(ctx context.Context, p httpkit.PageParams) ([]Material, int, error) {
	search := "%" + p.Search + "%"

	// Allow only known safe column names to avoid SQL injection via sort_by.
	sortCol := "created_at"
	switch p.SortBy {
	case "name", "type", "unit":
		sortCol = p.SortBy
	}
	orderDir := "ASC"
	if p.Order == "desc" {
		orderDir = "DESC"
	}

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM materials WHERE is_active = true AND name ILIKE $1`,
		search,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count materials: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, type, name, unit, is_active, min_remnant_length_mm, min_remnant_width_mm, created_at
		 FROM materials
		 WHERE is_active = true AND name ILIKE $1
		 ORDER BY %s %s
		 LIMIT $2 OFFSET $3`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, search, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Material
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.Type, &m.Name, &m.Unit, &m.IsActive,
			&m.MinRemnantLengthMM, &m.MinRemnantWidthMM, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

func (s *pgStore) selectMaterialByID(ctx context.Context, id uuid.UUID) (Material, error) {
	var m Material
	err := s.pool.QueryRow(ctx,
		`SELECT id, type, name, unit, is_active, min_remnant_length_mm, min_remnant_width_mm, created_at
		 FROM materials WHERE id = $1`,
		id,
	).Scan(&m.ID, &m.Type, &m.Name, &m.Unit, &m.IsActive,
		&m.MinRemnantLengthMM, &m.MinRemnantWidthMM, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Material{}, domain.ErrNotFound
		}
		return Material{}, err
	}
	return m, nil
}

func (s *pgStore) deactivateMaterial(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE materials SET is_active = false WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *pgStore) updateMinRemnantPolicy(ctx context.Context, id uuid.UUID, lengthMM, widthMM int) (Material, error) {
	var m Material
	err := s.pool.QueryRow(ctx,
		`UPDATE materials
		    SET min_remnant_length_mm = $2,
		        min_remnant_width_mm  = $3
		  WHERE id = $1 AND is_active = true
		  RETURNING id, type, name, unit, is_active, min_remnant_length_mm, min_remnant_width_mm, created_at`,
		id, lengthMM, widthMM,
	).Scan(&m.ID, &m.Type, &m.Name, &m.Unit, &m.IsActive,
		&m.MinRemnantLengthMM, &m.MinRemnantWidthMM, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Material{}, domain.ErrNotFound
		}
		return Material{}, err
	}
	return m, nil
}

func (s *pgStore) insertSKU(ctx context.Context, sku SKU) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO skus (id, code, name, length_mm, width_mm, requires_metal, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		sku.ID, sku.Code, sku.Name, sku.Dimensions.LengthMM, sku.Dimensions.WidthMM,
		sku.RequiresMetal, sku.IsActive, sku.CreatedAt,
	)
	return err
}

func (s *pgStore) selectSKUs(ctx context.Context) ([]SKU, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, length_mm, width_mm, requires_metal, is_active, created_at
		 FROM skus WHERE is_active = true ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SKU
	for rows.Next() {
		var sk SKU
		if err := rows.Scan(&sk.ID, &sk.Code, &sk.Name, &sk.Dimensions.LengthMM, &sk.Dimensions.WidthMM,
			&sk.RequiresMetal, &sk.IsActive, &sk.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// selectSKUsPaged returns a page of active SKUs optionally filtered by a
// case-insensitive keyword match on the name or code columns.
// It returns (items, totalMatchingItems, error).
func (s *pgStore) selectSKUsPaged(ctx context.Context, p httpkit.PageParams) ([]SKU, int, error) {
	search := "%" + p.Search + "%"

	// Allow only known safe column names to avoid SQL injection via sort_by.
	sortCol := "created_at"
	switch p.SortBy {
	case "name", "code":
		sortCol = p.SortBy
	}
	orderDir := "ASC"
	if p.Order == "desc" {
		orderDir = "DESC"
	}

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM skus WHERE is_active = true AND (name ILIKE $1 OR code ILIKE $1)`,
		search,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count skus: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, code, name, length_mm, width_mm, requires_metal, is_active, created_at
		 FROM skus
		 WHERE is_active = true AND (name ILIKE $1 OR code ILIKE $1)
		 ORDER BY %s %s
		 LIMIT $2 OFFSET $3`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, search, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []SKU
	for rows.Next() {
		var sku SKU
		if err := rows.Scan(&sku.ID, &sku.Code, &sku.Name,
			&sku.Dimensions.LengthMM, &sku.Dimensions.WidthMM,
			&sku.RequiresMetal, &sku.IsActive, &sku.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, sku)
	}
	return out, total, rows.Err()
}

func (s *pgStore) selectSKUByID(ctx context.Context, id uuid.UUID) (SKU, error) {
	var sku SKU
	err := s.pool.QueryRow(ctx,
		`SELECT id, code, name, length_mm, width_mm, requires_metal, is_active, created_at
		 FROM skus WHERE id = $1`,
		id,
	).Scan(&sku.ID, &sku.Code, &sku.Name, &sku.Dimensions.LengthMM, &sku.Dimensions.WidthMM,
		&sku.RequiresMetal, &sku.IsActive, &sku.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SKU{}, domain.ErrNotFound
		}
		return SKU{}, err
	}
	return sku, nil
}

func (s *pgStore) deactivateSKU(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE skus SET is_active = false WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *pgStore) upsertBOM(ctx context.Context, skuID uuid.UUID, components []BOMComponent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `DELETE FROM bom_components WHERE sku_id = $1`, skuID)
	if err != nil {
		return err
	}

	for _, c := range components {
		_, err = tx.Exec(ctx,
			`INSERT INTO bom_components (id, sku_id, material_id, material_type, quantity_per_unit, unit)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)`,
			skuID, c.MaterialID, c.MaterialType, c.QuantityPerUnit, c.Unit,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *pgStore) selectBOMBySKU(ctx context.Context, skuID uuid.UUID) (BOM, error) {
	var dummy uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM skus WHERE id = $1`, skuID).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BOM{}, domain.ErrNotFound
		}
		return BOM{}, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT material_id, material_type, quantity_per_unit, unit
		 FROM bom_components WHERE sku_id = $1`,
		skuID,
	)
	if err != nil {
		return BOM{}, err
	}
	defer rows.Close()

	var components []BOMComponent
	for rows.Next() {
		var c BOMComponent
		if err := rows.Scan(&c.MaterialID, &c.MaterialType, &c.QuantityPerUnit, &c.Unit); err != nil {
			return BOM{}, err
		}
		components = append(components, c)
	}
	if err := rows.Err(); err != nil {
		return BOM{}, err
	}

	return BOM{SKUID: skuID, Components: components}, nil
}

func (s *pgStore) insertBOMVariant(ctx context.Context, v BOMVariant, components []BOMComponent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO bom_variants (id, sku_id, variant_code, name, is_default, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		v.ID, v.SKUID, v.VariantCode, v.Name, v.IsDefault, v.CreatedAt,
	)
	if err != nil {
		return err
	}

	for _, c := range components {
		_, err = tx.Exec(ctx,
			`INSERT INTO bom_variant_components (id, variant_id, material_id, material_type, quantity_per_unit, unit)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)`,
			v.ID, c.MaterialID, c.MaterialType, c.QuantityPerUnit, c.Unit,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *pgStore) selectBOMVariantsBySkuID(ctx context.Context, skuID uuid.UUID) ([]BOMVariant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, sku_id, variant_code, name, is_default, created_at
		 FROM bom_variants WHERE sku_id = $1 ORDER BY is_default DESC, created_at ASC`,
		skuID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BOMVariant
	for rows.Next() {
		var v BOMVariant
		if err := rows.Scan(&v.ID, &v.SKUID, &v.VariantCode, &v.Name, &v.IsDefault, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *pgStore) selectBOMVariantByCode(ctx context.Context, skuID uuid.UUID, variantCode string) (BOMVariant, error) {
	var v BOMVariant
	err := s.pool.QueryRow(ctx,
		`SELECT id, sku_id, variant_code, name, is_default, created_at
		 FROM bom_variants WHERE sku_id = $1 AND variant_code = $2`,
		skuID, variantCode,
	).Scan(&v.ID, &v.SKUID, &v.VariantCode, &v.Name, &v.IsDefault, &v.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BOMVariant{}, domain.ErrNotFound
		}
		return BOMVariant{}, err
	}
	return v, nil
}

func (s *pgStore) selectBOMComponentsByVariantID(ctx context.Context, variantID uuid.UUID) ([]BOMComponent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT material_id, material_type, quantity_per_unit, unit
		 FROM bom_variant_components WHERE variant_id = $1`,
		variantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BOMComponent
	for rows.Next() {
		var c BOMComponent
		if err := rows.Scan(&c.MaterialID, &c.MaterialType, &c.QuantityPerUnit, &c.Unit); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
