package barcode

import (
	"context"
	"errors"

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

func (s *pgStore) selectBarcodesByIDsOrdered(ctx context.Context, ids []uuid.UUID) ([]Barcode, error) {
	if len(ids) == 0 {
		return []Barcode{}, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT b.id, b.work_order_id, b.sku_id, b.po_id, b.production_plan_id, b.sku_code, b.sku_name, b.dimensions, b.produced_date, b.created_at
		 FROM unnest($1::uuid[]) WITH ORDINALITY AS req(id, ord)
		 JOIN barcodes b ON b.id = req.id
		 ORDER BY req.ord`,
		ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Barcode, 0, len(ids))
	for rows.Next() {
		var b Barcode
		if err := rows.Scan(&b.ID, &b.WorkOrderID, &b.SKUID, &b.POID, &b.ProductionPlanID,
			&b.SKUCode, &b.SKUName, &b.Dimensions, &b.ProducedDate, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(ids) {
		return nil, domain.NewBizError(domain.ErrNotFound, "one or more barcodes not found")
	}
	return out, nil
}

func (s *pgStore) insertScanEvent(ctx context.Context, e ScanEvent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scan_events (id, barcode_id, checkpoint, scanned_by, location, note, device_id, device_name, shift, scanned_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10)`,
		e.ID, e.BarcodeID, e.Checkpoint, e.ScannedBy, e.Location, e.Note, e.DeviceID, e.DeviceName, e.Shift, e.ScannedAt,
	)
	return err
}

func (s *pgStore) selectScanEventsByBarcodeKeyset(ctx context.Context, barcodeID uuid.UUID, cur httpkit.Cursor, limit int) ([]ScanEvent, error) {
	// Two SQL shapes so the first-page case stays a clean prefix scan
	// rather than evaluating an always-true comparison. Index used in both:
	// idx_scan_events_barcode_scanned_at_id (barcode_id, scanned_at, id).
	const baseCols = `id, barcode_id, checkpoint, scanned_by, location, note, device_id, device_name, shift, scanned_at`

	var (
		rows pgx.Rows
		err  error
	)
	if cur.IsZero() {
		rows, err = s.pool.Query(ctx,
			`SELECT `+baseCols+`
			   FROM scan_events
			  WHERE barcode_id = $1
			  ORDER BY scanned_at, id
			  LIMIT $2`,
			barcodeID, limit,
		)
	} else {
		// Strict "after this row" predicate using the row constructor; the
		// composite index supports this comparison directly.
		rows, err = s.pool.Query(ctx,
			`SELECT `+baseCols+`
			   FROM scan_events
			  WHERE barcode_id = $1
			    AND (scanned_at, id) > ($2, $3)
			  ORDER BY scanned_at, id
			  LIMIT $4`,
			barcodeID, cur.Ts, cur.ID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScanEvent
	for rows.Next() {
		var (
			e          ScanEvent
			location   *string
			note       *string
			deviceID   *string
			deviceName *string
			shift      *string
		)
		if err := rows.Scan(&e.ID, &e.BarcodeID, &e.Checkpoint, &e.ScannedBy, &location, &note, &deviceID, &deviceName, &shift, &e.ScannedAt); err != nil {
			return nil, err
		}
		if location != nil {
			e.Location = *location
		}
		if note != nil {
			e.Note = *note
		}
		if deviceID != nil {
			e.DeviceID = *deviceID
		}
		if deviceName != nil {
			e.DeviceName = *deviceName
		}
		if shift != nil {
			e.Shift = *shift
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *pgStore) selectLastScanEventByBarcode(ctx context.Context, barcodeID uuid.UUID) (ScanEvent, error) {
	var (
		e          ScanEvent
		location   *string
		note       *string
		deviceID   *string
		deviceName *string
		shift      *string
	)
	err := s.pool.QueryRow(ctx,
		`SELECT id, barcode_id, checkpoint, scanned_by, location, note, device_id, device_name, shift, scanned_at
		 FROM scan_events WHERE barcode_id = $1
		 ORDER BY scanned_at DESC, id DESC LIMIT 1`,
		barcodeID,
	).Scan(&e.ID, &e.BarcodeID, &e.Checkpoint, &e.ScannedBy, &location, &note, &deviceID, &deviceName, &shift, &e.ScannedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScanEvent{}, domain.NewBizError(domain.ErrNotFound, "scan event not found")
		}
		return ScanEvent{}, err
	}
	if location != nil {
		e.Location = *location
	}
	if note != nil {
		e.Note = *note
	}
	if deviceID != nil {
		e.DeviceID = *deviceID
	}
	if deviceName != nil {
		e.DeviceName = *deviceName
	}
	if shift != nil {
		e.Shift = *shift
	}
	return e, nil
}
