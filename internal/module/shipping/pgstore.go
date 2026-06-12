package shipping

import (
	"context"
	"errors"
	"strconv"
	"strings"
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

// ── Vessel ───────────────────────────────────────────────────────────────────

func (s *pgStore) insertVessel(ctx context.Context, v Vessel) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO vessels
		    (id, name, carrier, voyage_number, etd, eta, cutoff_date,
		     port_of_loading, port_of_discharge, created_by, created_at)
		 VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11)`,
		v.ID, v.Name, v.Carrier, v.VoyageNumber, v.ETD, v.ETA, v.CutoffDate,
		v.PortOfLoading, v.PortOfDischarge, v.CreatedBy, v.CreatedAt,
	)
	return err
}

func (s *pgStore) getVessel(ctx context.Context, id uuid.UUID) (Vessel, error) {
	var v Vessel
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(carrier,''), COALESCE(voyage_number,''),
		        etd, eta, cutoff_date,
		        COALESCE(port_of_loading,''), COALESCE(port_of_discharge,''),
		        created_by, created_at
		 FROM vessels WHERE id = $1`, id,
	).Scan(
		&v.ID, &v.Name, &v.Carrier, &v.VoyageNumber,
		&v.ETD, &v.ETA, &v.CutoffDate,
		&v.PortOfLoading, &v.PortOfDischarge,
		&v.CreatedBy, &v.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Vessel{}, domain.NewBizError(domain.ErrNotFound, "vessel not found")
	}
	return v, err
}

func (s *pgStore) listVessels(ctx context.Context, p httpkit.PageParams, f VesselListFilter) (httpkit.PagedResult[Vessel], error) {
	var args []any
	var clauses []string

	addTime := func(col string, val *time.Time, op string) {
		if val == nil {
			return
		}
		args = append(args, *val)
		clauses = append(clauses, col+" "+op+" $"+strconv.Itoa(len(args)))
	}

	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		n := strconv.Itoa(len(args))
		clauses = append(clauses, "(name ILIKE $"+n+" OR COALESCE(voyage_number,'') ILIKE $"+n+")")
	}
	addTime("cutoff_date", f.CutoffFrom, ">=")
	addTime("cutoff_date", f.CutoffTo, "<")
	addTime("etd", f.ETDFrom, ">=")
	addTime("etd", f.ETDTo, "<")

	where := "TRUE"
	if len(clauses) > 0 {
		where = strings.Join(clauses, " AND ")
	}

	const cols = `SELECT id, name, COALESCE(carrier,''), COALESCE(voyage_number,''),
		        etd, eta, cutoff_date,
		        COALESCE(port_of_loading,''), COALESCE(port_of_discharge,''),
		        created_by, created_at
		 FROM vessels WHERE `

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM vessels WHERE `+where, args...,
	).Scan(&total); err != nil {
		return httpkit.PagedResult[Vessel]{}, err
	}

	args = append(args, p.Limit, p.Offset())
	limitArg := "$" + strconv.Itoa(len(args)-1)
	offsetArg := "$" + strconv.Itoa(len(args))

	rows, err := s.pool.Query(ctx,
		cols+where+` ORDER BY cutoff_date ASC, id ASC LIMIT `+limitArg+` OFFSET `+offsetArg,
		args...,
	)
	if err != nil {
		return httpkit.PagedResult[Vessel]{}, err
	}
	defer rows.Close()

	var vessels []Vessel
	for rows.Next() {
		var v Vessel
		if err := rows.Scan(
			&v.ID, &v.Name, &v.Carrier, &v.VoyageNumber,
			&v.ETD, &v.ETA, &v.CutoffDate,
			&v.PortOfLoading, &v.PortOfDischarge,
			&v.CreatedBy, &v.CreatedAt,
		); err != nil {
			return httpkit.PagedResult[Vessel]{}, err
		}
		vessels = append(vessels, v)
	}
	if err := rows.Err(); err != nil {
		return httpkit.PagedResult[Vessel]{}, err
	}

	return httpkit.NewPagedResult(vessels, total, p), nil
}

func (s *pgStore) updateVessel(ctx context.Context, in UpdateVesselInput) (Vessel, error) {
	var v Vessel
	err := s.pool.QueryRow(ctx,
		`UPDATE vessels
		 SET name = $2,
		     carrier = NULLIF($3,''),
		     voyage_number = NULLIF($4,''),
		     etd = $5,
		     eta = $6,
		     cutoff_date = $7,
		     port_of_loading = NULLIF($8,''),
		     port_of_discharge = NULLIF($9,'')
		 WHERE id = $1
		 RETURNING id, name, COALESCE(carrier,''), COALESCE(voyage_number,''),
		           etd, eta, cutoff_date,
		           COALESCE(port_of_loading,''), COALESCE(port_of_discharge,''),
		           created_by, created_at`,
		in.ID, in.Name, in.Carrier, in.VoyageNumber,
		in.ETD, in.ETA, in.CutoffDate,
		in.PortOfLoading, in.PortOfDischarge,
	).Scan(
		&v.ID, &v.Name, &v.Carrier, &v.VoyageNumber,
		&v.ETD, &v.ETA, &v.CutoffDate,
		&v.PortOfLoading, &v.PortOfDischarge,
		&v.CreatedBy, &v.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Vessel{}, domain.NewBizError(domain.ErrNotFound, "vessel not found")
	}
	// Also update cutoff_date on any containers booked to this vessel.
	if err == nil {
		if _, execErr := s.pool.Exec(ctx,
			`UPDATE containers SET cutoff_date = $1 WHERE vessel_id = $2`,
			in.CutoffDate, in.ID,
		); execErr != nil {
			return Vessel{}, execErr
		}
	}
	return v, err
}

func (s *pgStore) deleteVessel(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM vessels WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "vessel not found")
	}
	return nil
}

// ── Booking ───────────────────────────────────────────────────────────────────

func (s *pgStore) upsertBooking(ctx context.Context, b ShippingBooking, cutoffDate time.Time) (ShippingBooking, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ShippingBooking{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Upsert on container_id unique constraint.
	_, err = tx.Exec(ctx,
		`INSERT INTO shipping_bookings
		    (id, vessel_id, container_id, booking_ref, booked_by, booked_at, note)
		 VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,''))
		 ON CONFLICT (container_id) DO UPDATE
		   SET vessel_id   = EXCLUDED.vessel_id,
		       booking_ref = EXCLUDED.booking_ref,
		       booked_by   = EXCLUDED.booked_by,
		       booked_at   = EXCLUDED.booked_at,
		       note        = EXCLUDED.note`,
		b.ID, b.VesselID, b.ContainerID, b.BookingRef, b.BookedBy, b.BookedAt, b.Note,
	)
	if err != nil {
		return ShippingBooking{}, err
	}

	// Denormalize vessel_id + cutoff_date onto the container (BR-D08 source).
	if _, err = tx.Exec(ctx,
		`UPDATE containers SET vessel_id = $1, cutoff_date = $2 WHERE id = $3`,
		b.VesselID, cutoffDate, b.ContainerID,
	); err != nil {
		return ShippingBooking{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ShippingBooking{}, err
	}
	return b, nil
}

func (s *pgStore) deleteBooking(ctx context.Context, containerID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx,
		`DELETE FROM shipping_bookings WHERE container_id = $1`, containerID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "booking not found")
	}

	// Clear the denormalized fields on the container.
	if _, err = tx.Exec(ctx,
		`UPDATE containers SET vessel_id = NULL, cutoff_date = NULL WHERE id = $1`,
		containerID,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *pgStore) getBookingByContainer(ctx context.Context, containerID uuid.UUID) (ShippingBooking, error) {
	var b ShippingBooking
	err := s.pool.QueryRow(ctx,
		`SELECT sb.id, sb.vessel_id, sb.container_id,
		        COALESCE(sb.booking_ref,''), sb.booked_by, sb.booked_at,
		        COALESCE(sb.note,''),
		        v.name, v.cutoff_date
		 FROM shipping_bookings sb
		 JOIN vessels v ON v.id = sb.vessel_id
		 WHERE sb.container_id = $1`, containerID,
	).Scan(
		&b.ID, &b.VesselID, &b.ContainerID,
		&b.BookingRef, &b.BookedBy, &b.BookedAt,
		&b.Note,
		&b.VesselName, &b.CutoffDate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShippingBooking{}, domain.NewBizError(domain.ErrNotFound, "booking not found")
	}
	return b, err
}

func (s *pgStore) listBookingsByVessel(ctx context.Context, vesselID uuid.UUID) ([]ShippingBooking, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT sb.id, sb.vessel_id, sb.container_id,
		        COALESCE(sb.booking_ref,''), sb.booked_by, sb.booked_at,
		        COALESCE(sb.note,''),
		        v.name, v.cutoff_date
		 FROM shipping_bookings sb
		 JOIN vessels v ON v.id = sb.vessel_id
		 WHERE sb.vessel_id = $1
		 ORDER BY sb.booked_at ASC`, vesselID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ShippingBooking
	for rows.Next() {
		var b ShippingBooking
		if err := rows.Scan(
			&b.ID, &b.VesselID, &b.ContainerID,
			&b.BookingRef, &b.BookedBy, &b.BookedAt,
			&b.Note,
			&b.VesselName, &b.CutoffDate,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
