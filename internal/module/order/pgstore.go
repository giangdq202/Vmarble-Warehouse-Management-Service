package order

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

func (s *pgStore) insertPOWithItems(ctx context.Context, p PO, items []LineItem) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`INSERT INTO purchase_orders (id, code, expected_delivery, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		p.ID, p.Code, p.ExpectedDelivery, p.IsActive, p.CreatedAt,
	); err != nil {
		return err
	}

	for _, item := range items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO po_line_items (id, po_id, sku_id, quantity, selling_price_amount, selling_price_currency)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			item.ID, item.POID, item.SKUID, item.Quantity, item.SellingPrice.Amount, item.SellingPrice.Currency,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *pgStore) selectPOsPaged(ctx context.Context, p httpkit.PageParams, f POListFilter) ([]PO, int, error) {
	search := "%" + p.Search + "%"

	sortCol := "created_at"
	switch p.SortBy {
	case "code", "expected_delivery":
		sortCol = p.SortBy
	}
	orderDir := "DESC"
	if p.Order == "asc" {
		orderDir = "ASC"
	}

	// Typed-nil binding so the IS NULL branch fires when From/To are unset
	// (INSTINCTS: typed-nil params for optional SQL filters). Without this,
	// passing (*time.Time)(nil) directly works in pgx but reads less clearly
	// than letting any-typed nils flow through.
	var fromAny, toAny any
	if f.From != nil {
		fromAny = *f.From
	}
	if f.To != nil {
		toAny = *f.To
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM purchase_orders
		 WHERE is_active = true AND code ILIKE $1
		   AND ($2::timestamptz IS NULL OR created_at >= $2)
		   AND ($3::timestamptz IS NULL OR created_at <  $3)`,
		search, fromAny, toAny,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(
		`SELECT po.id, po.code, po.expected_delivery, po.is_active, po.created_at,
		        COALESCE(COUNT(li.id), 0) AS item_count,
		        COALESCE(SUM(li.quantity), 0) AS total_quantity,
		        COALESCE(COUNT(DISTINCT li.sku_id), 0) AS total_skus
		 FROM purchase_orders po
		 LEFT JOIN po_line_items li ON li.po_id = po.id
		 WHERE po.is_active = true AND po.code ILIKE $1
		   AND ($2::timestamptz IS NULL OR po.created_at >= $2)
		   AND ($3::timestamptz IS NULL OR po.created_at <  $3)
		 GROUP BY po.id, po.code, po.expected_delivery, po.is_active, po.created_at
		 ORDER BY po.%s %s
		 LIMIT $4 OFFSET $5`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, search, fromAny, toAny, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var pos []PO
	for rows.Next() {
		var po PO
		if err := rows.Scan(&po.ID, &po.Code, &po.ExpectedDelivery, &po.IsActive, &po.CreatedAt, &po.ItemCount, &po.TotalQuantity, &po.TotalSKUs); err != nil {
			return nil, 0, err
		}
		pos = append(pos, po)
	}
	return pos, total, rows.Err()
}

func (s *pgStore) selectPOByID(ctx context.Context, id uuid.UUID) (PO, error) {
	var p PO
	err := s.pool.QueryRow(ctx,
		`SELECT id, code, expected_delivery, is_active, created_at FROM purchase_orders WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.Code, &p.ExpectedDelivery, &p.IsActive, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PO{}, domain.ErrNotFound
		}
		return PO{}, err
	}
	return p, nil
}

func (s *pgStore) deactivatePO(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE purchase_orders SET is_active = false WHERE id = $1`,
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

func (s *pgStore) selectLineItemsByPO(ctx context.Context, poID uuid.UUID) ([]LineItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, po_id, sku_id, quantity, selling_price_amount, selling_price_currency
		 FROM po_line_items WHERE po_id = $1 ORDER BY id`,
		poID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []LineItem
	for rows.Next() {
		var item LineItem
		if err := rows.Scan(&item.ID, &item.POID, &item.SKUID, &item.Quantity,
			&item.SellingPrice.Amount, &item.SellingPrice.Currency); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *pgStore) selectLineItemsBySKU(ctx context.Context, skuID uuid.UUID) ([]LineItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, po_id, sku_id, quantity, selling_price_amount, selling_price_currency
		 FROM po_line_items WHERE sku_id = $1 ORDER BY id`,
		skuID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []LineItem
	for rows.Next() {
		var item LineItem
		if err := rows.Scan(&item.ID, &item.POID, &item.SKUID, &item.Quantity,
			&item.SellingPrice.Amount, &item.SellingPrice.Currency); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
