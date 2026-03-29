package catalog

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) store {
	return &pgStore{pool: pool}
}

func (s *pgStore) insertMaterial(ctx context.Context, m Material) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO materials (id, type, name, unit, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		m.ID, m.Type, m.Name, m.Unit, m.CreatedAt,
	)
	return err
}

func (s *pgStore) selectMaterials(ctx context.Context) ([]Material, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, type, name, unit, created_at FROM materials ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Material
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.Type, &m.Name, &m.Unit, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *pgStore) selectMaterialByID(ctx context.Context, id uuid.UUID) (Material, error) {
	var m Material
	err := s.pool.QueryRow(ctx,
		`SELECT id, type, name, unit, created_at FROM materials WHERE id = $1`,
		id,
	).Scan(&m.ID, &m.Type, &m.Name, &m.Unit, &m.CreatedAt)
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
		`INSERT INTO skus (id, code, name, length_mm, width_mm, requires_metal, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sku.ID, sku.Code, sku.Name, sku.Dimensions.LengthMM, sku.Dimensions.WidthMM,
		sku.RequiresMetal, sku.CreatedAt,
	)
	return err
}

func (s *pgStore) selectSKUs(ctx context.Context) ([]SKU, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, length_mm, width_mm, requires_metal, created_at
		 FROM skus ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SKU
	for rows.Next() {
		var s SKU
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Dimensions.LengthMM, &s.Dimensions.WidthMM,
			&s.RequiresMetal, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *pgStore) selectSKUByID(ctx context.Context, id uuid.UUID) (SKU, error) {
	var sku SKU
	err := s.pool.QueryRow(ctx,
		`SELECT id, code, name, length_mm, width_mm, requires_metal, created_at
		 FROM skus WHERE id = $1`,
		id,
	).Scan(&sku.ID, &sku.Code, &sku.Name, &sku.Dimensions.LengthMM, &sku.Dimensions.WidthMM,
		&sku.RequiresMetal, &sku.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SKU{}, domain.ErrNotFound
		}
		return SKU{}, err
	}
	return sku, nil
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
