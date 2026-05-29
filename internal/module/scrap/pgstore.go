package scrap

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) store {
	return &pgStore{pool: pool}
}

func (s *pgStore) insertScrapSale(ctx context.Context, sale ScrapSale) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scrap_sales (
			id, sale_date, material_id, quantity_kg, unit_price, currency,
			buyer_name, invoice_number, notes, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		sale.ID, sale.SaleDate, sale.MaterialID, sale.QuantityKG, sale.UnitPrice, sale.Currency,
		sale.BuyerName, sale.InvoiceNumber, sale.Notes, sale.CreatedBy, sale.CreatedAt,
	)
	return err
}

func (s *pgStore) selectScrapSalesKeyset(ctx context.Context, filter ListScrapSalesFilter, cursor httpkit.Cursor, limit int) ([]ScrapSale, error) {
	query := `
		SELECT id, sale_date, material_id, quantity_kg, unit_price, currency,
		       total_amount, buyer_name, invoice_number, notes, created_by, created_at
		FROM scrap_sales
		WHERE ($1::timestamptz IS NULL OR sale_date >= $1::date)
		  AND ($2::timestamptz IS NULL OR sale_date < $2::date)
		  AND ($3::uuid IS NULL OR material_id = $3)
		  AND (
		      $4::timestamptz IS NULL
		      OR created_at < $4
		      OR (created_at = $4 AND id > $5)
		  )
		ORDER BY created_at DESC, id ASC
		LIMIT $6
	`
	var cursorTs *time.Time
	var cursorID uuid.UUID
	if !cursor.IsZero() {
		cursorTs = &cursor.Ts
		cursorID = cursor.ID
	}

	rows, err := s.pool.Query(ctx, query, filter.From, filter.To, filter.MaterialID, cursorTs, cursorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ScrapSale, 0)
	for rows.Next() {
		var s ScrapSale
		if err := rows.Scan(
			&s.ID, &s.SaleDate, &s.MaterialID, &s.QuantityKG, &s.UnitPrice, &s.Currency,
			&s.TotalAmount, &s.BuyerName, &s.InvoiceNumber, &s.Notes, &s.CreatedBy, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
