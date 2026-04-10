package barcode

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

func (s *pgStore) insertBarcode(ctx context.Context, b Barcode) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO barcodes (id, work_order_id, sku_id, po_id, production_plan_id, sku_code, sku_name, dimensions, produced_date, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		b.ID, b.WorkOrderID, b.SKUID, b.POID, b.ProductionPlanID,
		b.SKUCode, b.SKUName, b.Dimensions, b.ProducedDate, b.CreatedAt,
	)
	return err
}

func (s *pgStore) selectBarcodeByID(ctx context.Context, id uuid.UUID) (Barcode, error) {
	var b Barcode
	err := s.pool.QueryRow(ctx,
		`SELECT id, work_order_id, sku_id, po_id, production_plan_id, sku_code, sku_name, dimensions, produced_date, created_at
		 FROM barcodes WHERE id = $1`,
		id,
	).Scan(&b.ID, &b.WorkOrderID, &b.SKUID, &b.POID, &b.ProductionPlanID,
		&b.SKUCode, &b.SKUName, &b.Dimensions, &b.ProducedDate, &b.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Barcode{}, domain.ErrNotFound
		}
		return Barcode{}, err
	}
	return b, nil
}

func (s *pgStore) selectBarcodesByWorkOrder(ctx context.Context, workOrderID uuid.UUID) ([]Barcode, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, work_order_id, sku_id, po_id, production_plan_id, sku_code, sku_name, dimensions, produced_date, created_at
		 FROM barcodes WHERE work_order_id = $1 ORDER BY created_at`,
		workOrderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Barcode
	for rows.Next() {
		var b Barcode
		if err := rows.Scan(&b.ID, &b.WorkOrderID, &b.SKUID, &b.POID, &b.ProductionPlanID,
			&b.SKUCode, &b.SKUName, &b.Dimensions, &b.ProducedDate, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *pgStore) insertScanEvent(ctx context.Context, e ScanEvent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scan_events (id, barcode_id, checkpoint, scanned_by, scanned_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		e.ID, e.BarcodeID, e.Checkpoint, e.ScannedBy, e.ScannedAt,
	)
	return err
}

func (s *pgStore) selectScanEventsByBarcode(ctx context.Context, barcodeID uuid.UUID) ([]ScanEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, barcode_id, checkpoint, scanned_by, scanned_at
		 FROM scan_events WHERE barcode_id = $1 ORDER BY scanned_at`,
		barcodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScanEvent
	for rows.Next() {
		var e ScanEvent
		if err := rows.Scan(&e.ID, &e.BarcodeID, &e.Checkpoint, &e.ScannedBy, &e.ScannedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
