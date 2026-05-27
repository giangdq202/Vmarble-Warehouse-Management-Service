package sales

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// ── Customer ─────────────────────────────────────────────────────────────────

func (s *pgStore) nextCustomerCode(ctx context.Context) (string, error) {
	var seq int64
	if err := s.pool.QueryRow(ctx, `SELECT nextval('customer_code_seq')`).Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("KH%03d", seq), nil
}

func (s *pgStore) insertCustomer(ctx context.Context, c Customer) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO customers (id, code, name, country_code, address, contact_person,
		                        contact_phone, contact_email, is_active, created_at)
		 VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,$10)`,
		c.ID, c.Code, c.Name, c.CountryCode, c.Address, c.ContactPerson,
		c.ContactPhone, c.ContactEmail, c.IsActive, c.CreatedAt,
	)
	return err
}

func (s *pgStore) selectCustomerByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	var c Customer
	var country, address, person, phone, email *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, code, name, country_code, address, contact_person,
		        contact_phone, contact_email, is_active, created_at
		   FROM customers WHERE id = $1`,
		id,
	).Scan(&c.ID, &c.Code, &c.Name, &country, &address, &person, &phone, &email,
		&c.IsActive, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Customer{}, domain.ErrNotFound
		}
		return Customer{}, err
	}
	c.CountryCode = stringFromPtr(country)
	c.Address = stringFromPtr(address)
	c.ContactPerson = stringFromPtr(person)
	c.ContactPhone = stringFromPtr(phone)
	c.ContactEmail = stringFromPtr(email)
	return c, nil
}

func (s *pgStore) selectCustomersPaged(ctx context.Context, p httpkit.PageParams, activeOnly bool) ([]Customer, int, error) {
	search := "%" + p.Search + "%"

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customers
		  WHERE ($1::bool = false OR is_active = true)
		    AND ($2::text = '' OR code ILIKE $2 OR name ILIKE $2)`,
		activeOnly, search,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, country_code, address, contact_person,
		        contact_phone, contact_email, is_active, created_at
		   FROM customers
		  WHERE ($1::bool = false OR is_active = true)
		    AND ($2::text = '' OR code ILIKE $2 OR name ILIKE $2)
		  ORDER BY created_at DESC, id DESC
		 LIMIT $3 OFFSET $4`,
		activeOnly, search, p.Limit, p.Offset(),
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Customer
	for rows.Next() {
		var c Customer
		var country, address, person, phone, email *string
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &country, &address, &person,
			&phone, &email, &c.IsActive, &c.CreatedAt); err != nil {
			return nil, 0, err
		}
		c.CountryCode = stringFromPtr(country)
		c.Address = stringFromPtr(address)
		c.ContactPerson = stringFromPtr(person)
		c.ContactPhone = stringFromPtr(phone)
		c.ContactEmail = stringFromPtr(email)
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (s *pgStore) updateCustomer(ctx context.Context, c Customer) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE customers SET
		    name = $2,
		    country_code = NULLIF($3,''),
		    address = NULLIF($4,''),
		    contact_person = NULLIF($5,''),
		    contact_phone = NULLIF($6,''),
		    contact_email = NULLIF($7,''),
		    is_active = $8
		  WHERE id = $1`,
		c.ID, c.Name, c.CountryCode, c.Address, c.ContactPerson,
		c.ContactPhone, c.ContactEmail, c.IsActive,
	)
	return err
}

func (s *pgStore) customerCodeExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM customers WHERE code = $1)`, code,
	).Scan(&exists)
	return exists, err
}

// ── Sales order ──────────────────────────────────────────────────────────────

func (s *pgStore) nextSOCode(ctx context.Context, now time.Time) (string, error) {
	dateKey := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var seq int
	// INSERT … ON CONFLICT DO UPDATE … RETURNING is atomic per row, so two
	// concurrent calls on the same date_key serialize on the row lock and
	// return distinct seq values.
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sales_order_code_counters (date_key, last_seq)
		 VALUES ($1, 1)
		 ON CONFLICT (date_key) DO UPDATE
		   SET last_seq = sales_order_code_counters.last_seq + 1
		 RETURNING last_seq`,
		dateKey,
	).Scan(&seq)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SO%04d%02d%02d-%03d", now.Year(), now.Month(), now.Day(), seq), nil
}

func (s *pgStore) insertSO(ctx context.Context, so SalesOrder) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sales_orders (id, code, customer_id, incoterm, port_of_loading,
		                          port_of_discharge, currency, status, expected_ship_date,
		                          note, created_by, created_at)
		 VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),$7,$8,$9,NULLIF($10,''),$11,$12)`,
		so.ID, so.Code, so.CustomerID, so.Incoterm, so.PortOfLoading, so.PortOfDischarge,
		so.Currency, so.Status, so.ExpectedShipDate, so.Note, so.CreatedBy, so.CreatedAt,
	)
	return err
}

func (s *pgStore) insertSOLines(ctx context.Context, lines []SalesOrderLine) error {
	for _, l := range lines {
		_, err := s.pool.Exec(ctx,
			`INSERT INTO sales_order_lines (id, sales_order_id, sku_id, qty_ordered,
			                                qty_planned, qty_shipped,
			                                unit_price_amount, unit_price_currency, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			l.ID, l.SalesOrderID, l.SKUID, l.QtyOrdered, l.QtyPlanned, l.QtyShipped,
			l.UnitPrice.Amount, l.UnitPrice.Currency, l.CreatedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *pgStore) deleteSOLinesBySO(ctx context.Context, soID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sales_order_lines WHERE sales_order_id = $1`, soID)
	return err
}

func (s *pgStore) selectSOByID(ctx context.Context, id uuid.UUID) (SalesOrder, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT so.id, so.code, so.customer_id, c.code, c.name, c.country_code,
		        so.incoterm, so.port_of_loading, so.port_of_discharge, so.currency,
		        so.status, so.expected_ship_date, so.note, so.created_by, so.created_at
		   FROM sales_orders so
		   JOIN customers c ON c.id = so.customer_id
		  WHERE so.id = $1`, id)
	out, err := scanSO(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SalesOrder{}, domain.ErrNotFound
		}
		return SalesOrder{}, err
	}
	return out, nil
}

func (s *pgStore) selectSOLinesBySOID(ctx context.Context, soID uuid.UUID) ([]SalesOrderLine, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, sales_order_id, sku_id, qty_ordered, qty_planned, qty_shipped,
		        unit_price_amount, unit_price_currency, created_at
		   FROM sales_order_lines WHERE sales_order_id = $1 ORDER BY created_at, id`,
		soID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SalesOrderLine
	for rows.Next() {
		var l SalesOrderLine
		if err := rows.Scan(&l.ID, &l.SalesOrderID, &l.SKUID, &l.QtyOrdered,
			&l.QtyPlanned, &l.QtyShipped, &l.UnitPrice.Amount, &l.UnitPrice.Currency,
			&l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *pgStore) selectSOsPaged(ctx context.Context, p httpkit.PageParams, f SOListFilter) ([]SalesOrder, int, error) {
	search := "%" + p.Search + "%"
	var customerID uuid.UUID
	if f.CustomerID != nil {
		customerID = *f.CustomerID
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM sales_orders so
		   JOIN customers c ON c.id = so.customer_id
		  WHERE ($1::text = '' OR so.status = $1)
		    AND ($2::uuid IS NULL OR so.customer_id = $2)
		    AND ($3::text = '' OR so.code ILIKE $3 OR c.code ILIKE $3 OR c.name ILIKE $3)`,
		f.Status, nullableUUID(customerID), search,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT so.id, so.code, so.customer_id, c.code, c.name, c.country_code,
		        so.incoterm, so.port_of_loading, so.port_of_discharge, so.currency,
		        so.status, so.expected_ship_date, so.note, so.created_by, so.created_at
		   FROM sales_orders so
		   JOIN customers c ON c.id = so.customer_id
		  WHERE ($1::text = '' OR so.status = $1)
		    AND ($2::uuid IS NULL OR so.customer_id = $2)
		    AND ($3::text = '' OR so.code ILIKE $3 OR c.code ILIKE $3 OR c.name ILIKE $3)
		  ORDER BY so.created_at DESC, so.id DESC
		 LIMIT $4 OFFSET $5`,
		f.Status, nullableUUID(customerID), search, p.Limit, p.Offset(),
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []SalesOrder
	for rows.Next() {
		so, err := scanSO(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, so)
	}
	return out, total, rows.Err()
}

func (s *pgStore) updateSO(ctx context.Context, so SalesOrder) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sales_orders SET
		    incoterm = NULLIF($2,''),
		    port_of_loading = NULLIF($3,''),
		    port_of_discharge = NULLIF($4,''),
		    currency = $5,
		    expected_ship_date = $6,
		    note = NULLIF($7,'')
		  WHERE id = $1`,
		so.ID, so.Incoterm, so.PortOfLoading, so.PortOfDischarge,
		so.Currency, so.ExpectedShipDate, so.Note,
	)
	return err
}

func (s *pgStore) updateSOStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE sales_orders SET status = $1 WHERE id = $2`, status, id)
	return err
}

// ── Cross-module hooks (delivery → sales) ────────────────────────────────────

func (s *pgStore) selectSOLineByID(ctx context.Context, soLineID uuid.UUID) (SalesOrderLine, SalesOrder, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT l.id, l.sales_order_id, l.sku_id, l.qty_ordered, l.qty_planned, l.qty_shipped,
		        l.unit_price_amount, l.unit_price_currency, l.created_at,
		        so.id, so.code, so.customer_id, c.code, c.name, c.country_code,
		        so.incoterm, so.port_of_loading, so.port_of_discharge, so.currency,
		        so.status, so.expected_ship_date, so.note, so.created_by, so.created_at
		   FROM sales_order_lines l
		   JOIN sales_orders so ON so.id = l.sales_order_id
		   JOIN customers c     ON c.id  = so.customer_id
		  WHERE l.id = $1`, soLineID)
	var l SalesOrderLine
	var so SalesOrder
	var country, incoterm, portLoad, portDisch, note *string
	if err := row.Scan(
		&l.ID, &l.SalesOrderID, &l.SKUID, &l.QtyOrdered, &l.QtyPlanned, &l.QtyShipped,
		&l.UnitPrice.Amount, &l.UnitPrice.Currency, &l.CreatedAt,
		&so.ID, &so.Code, &so.CustomerID, &so.CustomerCode, &so.CustomerName, &country,
		&incoterm, &portLoad, &portDisch, &so.Currency,
		&so.Status, &so.ExpectedShipDate, &note, &so.CreatedBy, &so.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SalesOrderLine{}, SalesOrder{}, domain.ErrNotFound
		}
		return SalesOrderLine{}, SalesOrder{}, err
	}
	so.CustomerCountry = stringFromPtr(country)
	so.Incoterm = stringFromPtr(incoterm)
	so.PortOfLoading = stringFromPtr(portLoad)
	so.PortOfDischarge = stringFromPtr(portDisch)
	so.Note = stringFromPtr(note)
	return l, so, nil
}

// recordShipmentTx walks each item, locks the sales_order_lines row FOR
// UPDATE inside the caller's tx, bumps qty_shipped, and recomputes the
// parent SO's status when every line of that SO has been satisfied. The
// chk_qty_shipped_le_planned CHECK is the authoritative backstop; we
// translate the violation to ErrInvalidInput so the API surface returns 400.
func (s *pgStore) recordShipmentTx(ctx context.Context, tx pgx.Tx, items []ShipmentItemInput) error {
	if len(items) == 0 {
		return nil
	}

	// Track which SOs we touched so we can recompute their status in one
	// place after the per-line updates land.
	soIDs := make(map[uuid.UUID]struct{}, len(items))

	for _, it := range items {
		if it.Qty <= 0 {
			return domain.NewBizError(domain.ErrInvalidInput, "shipment qty must be > 0")
		}
		var soID uuid.UUID
		// SELECT FOR UPDATE so two concurrent seals can't race on the same line.
		if err := tx.QueryRow(ctx,
			`SELECT sales_order_id FROM sales_order_lines WHERE id = $1 FOR UPDATE`,
			it.SOLineID).Scan(&soID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NewBizError(domain.ErrNotFound,
					"sales order line not found: "+it.SOLineID.String())
			}
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE sales_order_lines SET qty_shipped = qty_shipped + $2 WHERE id = $1`,
			it.SOLineID, it.Qty,
		); err != nil {
			if strings.Contains(err.Error(), "chk_qty_shipped_le_planned") {
				return domain.NewBizError(domain.ErrInvalidInput,
					"qty_shipped would exceed qty_planned for line "+it.SOLineID.String())
			}
			return err
		}
		soIDs[soID] = struct{}{}
	}

	for soID := range soIDs {
		if err := recomputeSOStatusTx(ctx, tx, soID); err != nil {
			return err
		}
	}
	return nil
}

// recomputeSOStatusTx flips the parent SO into PARTIALLY_SHIPPED or SHIPPED
// depending on how qty_shipped now compares to qty_ordered across all of its
// lines. Status is moved monotonically — we never leave SHIPPED, and we
// never go backward from PARTIALLY_SHIPPED to IN_PRODUCTION just because no
// line is fully shipped yet.
func recomputeSOStatusTx(ctx context.Context, tx pgx.Tx, soID uuid.UUID) error {
	var anyShipped, allShipped bool
	if err := tx.QueryRow(ctx,
		`SELECT
		   bool_or(qty_shipped > 0),
		   bool_and(qty_shipped >= qty_ordered)
		 FROM sales_order_lines WHERE sales_order_id = $1`,
		soID).Scan(&anyShipped, &allShipped); err != nil {
		return err
	}
	if allShipped {
		_, err := tx.Exec(ctx,
			`UPDATE sales_orders SET status = $2 WHERE id = $1 AND status <> $2`,
			soID, SOStatusShipped)
		return err
	}
	if anyShipped {
		// Don't downgrade a SHIPPED order; only flip into PARTIALLY_SHIPPED
		// from earlier statuses.
		_, err := tx.Exec(ctx,
			`UPDATE sales_orders
			    SET status = $2
			  WHERE id = $1
			    AND status IN ($3, $4, $5)`,
			soID, SOStatusPartiallyShipped,
			SOStatusConfirmed, SOStatusInProduction, SOStatusPartiallyShipped)
		return err
	}
	return nil
}

// ── Transaction support ──────────────────────────────────────────────────────

func (s *pgStore) withTx(ctx context.Context, fn func(tx txStore) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(&pgTxStore{tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgTxStore struct{ tx pgx.Tx }

func (t *pgTxStore) lockSOForUpdate(ctx context.Context, id uuid.UUID) (SalesOrder, error) {
	row := t.tx.QueryRow(ctx,
		`SELECT so.id, so.code, so.customer_id, c.code, c.name, c.country_code,
		        so.incoterm, so.port_of_loading, so.port_of_discharge, so.currency,
		        so.status, so.expected_ship_date, so.note, so.created_by, so.created_at
		   FROM sales_orders so
		   JOIN customers c ON c.id = so.customer_id
		  WHERE so.id = $1
		  FOR UPDATE OF so`,
		id)
	out, err := scanSO(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SalesOrder{}, domain.ErrNotFound
		}
		return SalesOrder{}, err
	}
	return out, nil
}

func (t *pgTxStore) lockAndReadSOLines(ctx context.Context, lineIDs []uuid.UUID) ([]SalesOrderLine, error) {
	if len(lineIDs) == 0 {
		return nil, nil
	}
	rows, err := t.tx.Query(ctx,
		`SELECT id, sales_order_id, sku_id, qty_ordered, qty_planned, qty_shipped,
		        unit_price_amount, unit_price_currency, created_at
		   FROM sales_order_lines
		  WHERE id = ANY($1)
		  FOR UPDATE`,
		lineIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	got := make(map[uuid.UUID]SalesOrderLine, len(lineIDs))
	for rows.Next() {
		var l SalesOrderLine
		if err := rows.Scan(&l.ID, &l.SalesOrderID, &l.SKUID, &l.QtyOrdered,
			&l.QtyPlanned, &l.QtyShipped, &l.UnitPrice.Amount, &l.UnitPrice.Currency,
			&l.CreatedAt); err != nil {
			return nil, err
		}
		got[l.ID] = l
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]SalesOrderLine, 0, len(lineIDs))
	for _, id := range lineIDs {
		l, ok := got[id]
		if !ok {
			return nil, domain.NewBizError(domain.ErrNotFound, "sales order line not found: "+id.String())
		}
		out = append(out, l)
	}
	return out, nil
}

func (t *pgTxStore) incrementQtyPlanned(ctx context.Context, lineID uuid.UUID, delta int) error {
	_, err := t.tx.Exec(ctx,
		`UPDATE sales_order_lines SET qty_planned = qty_planned + $2 WHERE id = $1`,
		lineID, delta,
	)
	if err != nil {
		// 23514 = check_violation. The chk_qty_planned_le_ordered CHECK fires
		// when a concurrent split slipped past our FOR UPDATE — surface the
		// violation as ErrInvalidInput so the API returns 400 not 500.
		if strings.Contains(err.Error(), "chk_qty_planned_le_ordered") {
			return domain.NewBizError(domain.ErrInvalidInput,
				"qty_planned would exceed qty_ordered for line "+lineID.String())
		}
		return err
	}
	return nil
}

func (t *pgTxStore) updateStatusIfCurrent(ctx context.Context, id uuid.UUID, expected []string, target string) (bool, error) {
	tag, err := t.tx.Exec(ctx,
		`UPDATE sales_orders SET status = $1 WHERE id = $2 AND status = ANY($3)`,
		target, id, expected,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// rowScanner abstracts pgx.Row and pgx.Rows for the shared SO scan path.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSO(r rowScanner) (SalesOrder, error) {
	var so SalesOrder
	var country, incoterm, portLoad, portDisch, note *string
	if err := r.Scan(
		&so.ID, &so.Code, &so.CustomerID, &so.CustomerCode, &so.CustomerName, &country,
		&incoterm, &portLoad, &portDisch, &so.Currency,
		&so.Status, &so.ExpectedShipDate, &note, &so.CreatedBy, &so.CreatedAt,
	); err != nil {
		return SalesOrder{}, err
	}
	so.CustomerCountry = stringFromPtr(country)
	so.Incoterm = stringFromPtr(incoterm)
	so.PortOfLoading = stringFromPtr(portLoad)
	so.PortOfDischarge = stringFromPtr(portDisch)
	so.Note = stringFromPtr(note)
	return so, nil
}

func stringFromPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// nullableUUID converts uuid.Nil to nil for use with $N::uuid IS NULL filters
// so the same query shape covers both "all customers" and "this customer".
func nullableUUID(id uuid.UUID) interface{} {
	if id == uuid.Nil {
		return nil
	}
	return id
}

// ── Customer SKU mappings (#304) ─────────────────────────────────────────────

// scanCustomerSKUMapping maps one row using the canonical column order.
func scanCustomerSKUMapping(r rowScanner) (CustomerSKUMapping, error) {
	var m CustomerSKUMapping
	var notes *string
	var createdBy *uuid.UUID
	if err := r.Scan(
		&m.CustomerID, &m.CustomerSKUCode, &m.SKUID,
		&notes, &createdBy, &m.CreatedAt, &m.UpdatedAt,
	); err != nil {
		return CustomerSKUMapping{}, err
	}
	m.Notes = stringFromPtr(notes)
	m.CreatedBy = createdBy
	return m, nil
}

const selectCSMCols = `customer_id, customer_sku_code, sku_id, notes, created_by, created_at, updated_at`

// mapCSMPgError translates the postgres error codes raised by inserts/updates
// against customer_sku_mappings into domain sentinel errors. Centralised so
// the bulk path and the single-row path return the same shape.
//
//	23505 unique_violation       → ErrInvalidInput (PK collision = BR-CSM02)
//	23503 foreign_key_violation  → ErrInvalidInput (unknown customer or sku)
func mapCSMPgError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.NewBizError(domain.ErrInvalidInput,
				"customer_sku mapping already exists for (customer_id, customer_sku_code)")
		case "23503":
			// constraint_name lets us tell customer FK from sku FK so the
			// caller renders a precise toast.
			if strings.Contains(pgErr.ConstraintName, "sku") {
				return domain.NewBizError(domain.ErrInvalidInput, "sku_id does not exist")
			}
			if strings.Contains(pgErr.ConstraintName, "customer") {
				return domain.NewBizError(domain.ErrInvalidInput, "customer_id does not exist")
			}
			return domain.NewBizError(domain.ErrInvalidInput, "foreign key violation")
		}
	}
	return err
}

func (s *pgStore) insertCustomerSKUMapping(ctx context.Context, m CustomerSKUMapping) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO customer_sku_mappings
		    (customer_id, customer_sku_code, sku_id, notes, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7)`,
		m.CustomerID, m.CustomerSKUCode, m.SKUID, m.Notes, m.CreatedBy, m.CreatedAt, m.UpdatedAt,
	)
	return mapCSMPgError(err)
}

func (s *pgStore) selectCustomerSKUMappingsPaged(ctx context.Context, p httpkit.PageParams, f CustomerSKUMappingFilter) ([]CustomerSKUMapping, int, error) {
	var customerID any
	if f.CustomerID != nil {
		customerID = *f.CustomerID
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customer_sku_mappings
		 WHERE ($1::uuid IS NULL OR customer_id = $1)`,
		customerID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+selectCSMCols+`
		 FROM customer_sku_mappings
		 WHERE ($1::uuid IS NULL OR customer_id = $1)
		 ORDER BY customer_id, customer_sku_code
		 LIMIT $2 OFFSET $3`,
		customerID, p.Limit, p.Offset(),
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]CustomerSKUMapping, 0)
	for rows.Next() {
		m, err := scanCustomerSKUMapping(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

func (s *pgStore) selectCustomerSKUMappingByPK(ctx context.Context, customerID uuid.UUID, code string) (CustomerSKUMapping, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+selectCSMCols+`
		 FROM customer_sku_mappings
		 WHERE customer_id = $1 AND customer_sku_code = $2`,
		customerID, code,
	)
	m, err := scanCustomerSKUMapping(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CustomerSKUMapping{}, domain.ErrNotFound
		}
		return CustomerSKUMapping{}, err
	}
	return m, nil
}

func (s *pgStore) updateCustomerSKUMapping(ctx context.Context, m CustomerSKUMapping) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE customer_sku_mappings
		    SET sku_id = $3, notes = NULLIF($4,''), updated_at = $5
		  WHERE customer_id = $1 AND customer_sku_code = $2`,
		m.CustomerID, m.CustomerSKUCode, m.SKUID, m.Notes, m.UpdatedAt,
	)
	if err != nil {
		return mapCSMPgError(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *pgStore) deleteCustomerSKUMapping(ctx context.Context, customerID uuid.UUID, code string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM customer_sku_mappings
		  WHERE customer_id = $1 AND customer_sku_code = $2`,
		customerID, code,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// bulkInsertCustomerSKUMappings inserts every row in one transaction. Any PK
// or FK violation aborts the whole batch (fail-all matches BR-D08). The
// caller validates input shape; this method is responsible only for the
// atomicity guarantee.
func (s *pgStore) bulkInsertCustomerSKUMappings(ctx context.Context, rows []CustomerSKUMapping) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, m := range rows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO customer_sku_mappings
			    (customer_id, customer_sku_code, sku_id, notes, created_by, created_at, updated_at)
			 VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7)`,
			m.CustomerID, m.CustomerSKUCode, m.SKUID, m.Notes, m.CreatedBy, m.CreatedAt, m.UpdatedAt,
		); err != nil {
			return mapCSMPgError(err)
		}
	}
	return tx.Commit(ctx)
}
