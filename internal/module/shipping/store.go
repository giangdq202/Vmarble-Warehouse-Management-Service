package shipping

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertVessel(ctx context.Context, v Vessel) error
	getVessel(ctx context.Context, id uuid.UUID) (Vessel, error)
	listVessels(ctx context.Context, p httpkit.PageParams, f VesselListFilter) (httpkit.PagedResult[Vessel], error)
	updateVessel(ctx context.Context, in UpdateVesselInput) (Vessel, error)
	deleteVessel(ctx context.Context, id uuid.UUID) error

	// upsertBooking creates or replaces the booking for a container.
	// Also writes vessel_id + cutoff_date onto containers (BR-D08 source of truth).
	upsertBooking(ctx context.Context, b ShippingBooking, cutoffDate time.Time) (ShippingBooking, error)
	deleteBooking(ctx context.Context, containerID uuid.UUID) error
	getBookingByContainer(ctx context.Context, containerID uuid.UUID) (ShippingBooking, error)
	listBookingsByVessel(ctx context.Context, vesselID uuid.UUID) ([]ShippingBooking, error)
}
