package purchasing

import (
	"context"
	"errors"
	"fmt"
	"time"

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

const poSelectCols = `id, code, material_id, supplier, status, note, ordered_at, received_at, created_by, created_at`

func scanPO(row interface{ Scan(...any) error }) (PurchaseOrder, error) {
	var po PurchaseOrder
	err := row.Scan(
		&po.ID, &po.Code, &po.MaterialID, &po.Supplier,
		&po.Status, &po.Note, &po.OrderedAt, &po.ReceivedAt,
		&po.CreatedBy, &po.CreatedAt,
	)
	return po, err
}

func (s *pgStore) insertPO(ctx context.Context, po PurchaseOrder) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO material_purchase_orders
		 (id, code, material_id, supplier, status, note, ordered_at, received_at, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		po.ID, po.Code, po.MaterialID, po.Supplier, po.Status, po.Note,
		po.OrderedAt, po.ReceivedAt, po.CreatedBy, po.CreatedAt,
	)
	return err
}

func (s *pgStore) selectPOByID(ctx context.Context, id uuid.UUID) (PurchaseOrder, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+poSelectCols+` FROM material_purchase_orders WHERE id = $1`, id)
	po, err := scanPO(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseOrder{}, domain.ErrNotFound
		}
		return PurchaseOrder{}, err
	}
	items, err := s.selectPOItems(ctx, po.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	po.Items = items
	return po, nil
}

func (s *pgStore) selectPOsPaged(ctx context.Context, p httpkit.PageParams, f POListFilter) ([]PurchaseOrder, int, error) {
	orderDir := "DESC"
	if p.Order == "asc" {
		orderDir = "ASC"
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM material_purchase_orders
		 WHERE ($1::text = '' OR status = $1)
		   AND ($2::uuid IS NULL OR material_id = $2)`,
		f.Status, f.MaterialID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(
		`SELECT `+poSelectCols+`
		 FROM material_purchase_orders
		 WHERE ($1::text = '' OR status = $1)
		   AND ($2::uuid IS NULL OR material_id = $2)
		 ORDER BY created_at %s
		 LIMIT $3 OFFSET $4`,
		orderDir,
	)
	rows, err := s.pool.Query(ctx, query, f.Status, f.MaterialID, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []PurchaseOrder
	for rows.Next() {
		po, err := scanPO(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, po)
	}
	return out, total, rows.Err()
}

func (s *pgStore) updatePOStatus(ctx context.Context, id uuid.UUID, status POStatus, ts *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE material_purchase_orders SET status = $2,
		 ordered_at  = CASE WHEN $2 = 'ORDERED'   THEN $3 ELSE ordered_at  END,
		 received_at = CASE WHEN $2 = 'RECEIVED'  THEN $3 ELSE received_at END
		 WHERE id = $1`,
		id, string(status), ts,
	)
	return err
}

const itemSelectCols = `id, po_id, quantity, length_mm, width_mm,
	unit_cost_amount, unit_cost_currency, lot_id, created_at`

func scanItem(row interface{ Scan(...any) error }) (POItem, error) {
	var item POItem
	err := row.Scan(
		&item.ID, &item.POID, &item.Quantity, &item.LengthMM, &item.WidthMM,
		&item.UnitCost.Amount, &item.UnitCost.Currency,
		&item.LotID, &item.CreatedAt,
	)
	return item, err
}

func (s *pgStore) insertPOItem(ctx context.Context, item POItem) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO material_purchase_order_items
		 (id, po_id, quantity, length_mm, width_mm, unit_cost_amount, unit_cost_currency, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		item.ID, item.POID, item.Quantity, item.LengthMM, item.WidthMM,
		item.UnitCost.Amount, item.UnitCost.Currency, item.CreatedAt,
	)
	return err
}

func (s *pgStore) deletePOItem(ctx context.Context, poID, itemID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM material_purchase_order_items WHERE id = $1 AND po_id = $2`,
		itemID, poID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *pgStore) selectPOItems(ctx context.Context, poID uuid.UUID) ([]POItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+itemSelectCols+` FROM material_purchase_order_items
		 WHERE po_id = $1 ORDER BY created_at`, poID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []POItem
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *pgStore) linkItemToLot(ctx context.Context, itemID, lotID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE material_purchase_order_items SET lot_id = $2 WHERE id = $1`,
		itemID, lotID,
	)
	return err
}
