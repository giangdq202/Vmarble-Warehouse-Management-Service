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

func (s *pgStore) insertPO(ctx context.Context, p PO) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO purchase_orders (id, code, expected_delivery, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		p.ID, p.Code, p.ExpectedDelivery, p.IsActive, p.CreatedAt,
	)
	return err
}

func (s *pgStore) selectPOsPaged(ctx context.Context, p httpkit.PageParams) ([]PO, int, error) {
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

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM purchase_orders WHERE is_active = true AND code ILIKE $1`,
		search,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(
		`SELECT id, code, expected_delivery, is_active, created_at
		 FROM purchase_orders
		 WHERE is_active = true AND code ILIKE $1
		 ORDER BY %s %s
		 LIMIT $2 OFFSET $3`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, search, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var pos []PO
	for rows.Next() {
		var po PO
		if err := rows.Scan(&po.ID, &po.Code, &po.ExpectedDelivery, &po.IsActive, &po.CreatedAt); err != nil {
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

func (s *pgStore) insertLineItems(ctx context.Context, items []LineItem) error {
	for _, item := range items {
		_, err := s.pool.Exec(ctx,
			`INSERT INTO po_line_items (id, po_id, sku_id, quantity, selling_price_amount, selling_price_currency)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			item.ID, item.POID, item.SKUID, item.Quantity, item.SellingPrice.Amount, item.SellingPrice.Currency,
		)
		if err != nil {
			return err
		}
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
