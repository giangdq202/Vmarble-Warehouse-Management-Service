package shipping

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s   store
	now func() time.Time
}

func NewService(s store) Service {
	return &service{s: s, now: time.Now}
}

func (svc *service) CreateVessel(ctx context.Context, in CreateVesselInput) (Vessel, error) {
	if in.Name == "" {
		return Vessel{}, domain.NewBizError(domain.ErrInvalidInput, "vessel name is required")
	}
	if in.CutoffDate.IsZero() {
		return Vessel{}, domain.NewBizError(domain.ErrInvalidInput, "cutoff_date is required")
	}
	if in.CreatedBy == uuid.Nil {
		return Vessel{}, domain.NewBizError(domain.ErrInvalidInput, "created_by is required")
	}
	v := Vessel{
		ID:              uuid.New(),
		Name:            in.Name,
		Carrier:         in.Carrier,
		VoyageNumber:    in.VoyageNumber,
		ETD:             in.ETD,
		ETA:             in.ETA,
		CutoffDate:      in.CutoffDate.UTC(),
		PortOfLoading:   in.PortOfLoading,
		PortOfDischarge: in.PortOfDischarge,
		CreatedBy:       in.CreatedBy,
		CreatedAt:       svc.now().UTC(),
	}
	if err := svc.s.insertVessel(ctx, v); err != nil {
		return Vessel{}, err
	}
	return v, nil
}

func (svc *service) GetVessel(ctx context.Context, id uuid.UUID) (Vessel, error) {
	return svc.s.getVessel(ctx, id)
}

func (svc *service) ListVessels(ctx context.Context, p httpkit.PageParams, f VesselListFilter) (httpkit.PagedResult[Vessel], error) {
	return svc.s.listVessels(ctx, p, f)
}

func (svc *service) UpdateVessel(ctx context.Context, in UpdateVesselInput) (Vessel, error) {
	if in.ID == uuid.Nil {
		return Vessel{}, domain.NewBizError(domain.ErrInvalidInput, "vessel id is required")
	}
	if in.Name == "" {
		return Vessel{}, domain.NewBizError(domain.ErrInvalidInput, "vessel name is required")
	}
	if in.CutoffDate.IsZero() {
		return Vessel{}, domain.NewBizError(domain.ErrInvalidInput, "cutoff_date is required")
	}
	return svc.s.updateVessel(ctx, in)
}

func (svc *service) DeleteVessel(ctx context.Context, id uuid.UUID) error {
	return svc.s.deleteVessel(ctx, id)
}

func (svc *service) BookContainer(ctx context.Context, in BookContainerInput) (ShippingBooking, error) {
	if in.VesselID == uuid.Nil {
		return ShippingBooking{}, domain.NewBizError(domain.ErrInvalidInput, "vessel_id is required")
	}
	if in.ContainerID == uuid.Nil {
		return ShippingBooking{}, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	if in.BookedBy == uuid.Nil {
		return ShippingBooking{}, domain.NewBizError(domain.ErrInvalidInput, "booked_by is required")
	}

	v, err := svc.s.getVessel(ctx, in.VesselID)
	if err != nil {
		return ShippingBooking{}, err
	}

	b := ShippingBooking{
		ID:          uuid.New(),
		VesselID:    in.VesselID,
		ContainerID: in.ContainerID,
		BookingRef:  in.BookingRef,
		BookedBy:    in.BookedBy,
		BookedAt:    svc.now().UTC(),
		Note:        in.Note,
		VesselName:  v.Name,
		CutoffDate:  v.CutoffDate,
	}
	return svc.s.upsertBooking(ctx, b, v.CutoffDate)
}

func (svc *service) UnbookContainer(ctx context.Context, vesselID, containerID uuid.UUID) error {
	// Verify the booking actually belongs to this vessel before deleting.
	existing, err := svc.s.getBookingByContainer(ctx, containerID)
	if err != nil {
		return err
	}
	if existing.VesselID != vesselID {
		return domain.NewBizError(domain.ErrNotFound, "booking not found for this vessel/container pair")
	}
	return svc.s.deleteBooking(ctx, containerID)
}

func (svc *service) GetContainerBooking(ctx context.Context, containerID uuid.UUID) (ShippingBooking, error) {
	return svc.s.getBookingByContainer(ctx, containerID)
}

func (svc *service) ListContainerBookings(ctx context.Context, vesselID uuid.UUID) ([]ShippingBooking, error) {
	return svc.s.listBookingsByVessel(ctx, vesselID)
}
