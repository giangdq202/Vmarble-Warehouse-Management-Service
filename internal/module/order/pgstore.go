package order

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

func (s *pgStore) insertPO(ctx context.Context, p PO) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO purchase_orders (id, code, expected_delivery, created_at)
		 VALUES ($1, $2, $3, $4)`,
		p.ID, p.Code, p.ExpectedDelivery, p.CreatedAt,
	)
	return err
}

func (s *pgStore) selectPOs(ctx context.Context) ([]PO, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, expected_delivery, created_at FROM purchase_orders ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pos []PO
	for rows.Next() {
		var p PO
		if err := rows.Scan(&p.ID, &p.Code, &p.ExpectedDelivery, &p.CreatedAt); err != nil {
			return nil, err
		}
		pos = append(pos, p)
	}
	return pos, rows.Err()
}

func (s *pgStore) selectPOByID(ctx context.Context, id uuid.UUID) (PO, error) {
	var p PO
	err := s.pool.QueryRow(ctx,
		`SELECT id, code, expected_delivery, created_at FROM purchase_orders WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.Code, &p.ExpectedDelivery, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PO{}, domain.ErrNotFound
		}
		return PO{}, err
	}
	return p, nil
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
