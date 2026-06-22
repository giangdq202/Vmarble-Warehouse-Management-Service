package shipping

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mock store ────────────────────────────────────────────────────────────────

type mockStore struct {
	vessel   Vessel
	booking  ShippingBooking
	vesselErr  error
	bookingErr error
}

func (m *mockStore) insertVessel(_ context.Context, v Vessel) error {
	if m.vesselErr != nil {
		return m.vesselErr
	}
	m.vessel = v
	return nil
}
func (m *mockStore) getVessel(_ context.Context, _ uuid.UUID) (Vessel, error) {
	return m.vessel, m.vesselErr
}
func (m *mockStore) listVessels(_ context.Context, _ httpkit.PageParams, _ VesselListFilter) (httpkit.PagedResult[Vessel], error) {
	return httpkit.PagedResult[Vessel]{}, nil
}
func (m *mockStore) updateVessel(_ context.Context, in UpdateVesselInput) (Vessel, error) {
	if m.vesselErr != nil {
		return Vessel{}, m.vesselErr
	}
	return Vessel{ID: in.ID, Name: in.Name, CutoffDate: in.CutoffDate}, nil
}
func (m *mockStore) deleteVessel(_ context.Context, _ uuid.UUID) error { return m.vesselErr }
func (m *mockStore) upsertBooking(_ context.Context, b ShippingBooking, _ time.Time) (ShippingBooking, error) {
	return b, m.bookingErr
}
func (m *mockStore) deleteBooking(_ context.Context, _ uuid.UUID) error { return m.bookingErr }
func (m *mockStore) getBookingByContainer(_ context.Context, _ uuid.UUID) (ShippingBooking, error) {
	return m.booking, m.bookingErr
}
func (m *mockStore) listBookingsByVessel(_ context.Context, _ uuid.UUID) ([]ShippingBooking, error) {
	return nil, nil
}

var _ store = (*mockStore)(nil)

var fixedNow = time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)

func newSvc(ms *mockStore) *service {
	return &service{s: ms, now: func() time.Time { return fixedNow }}
}

// ── CreateVessel ──────────────────────────────────────────────────────────────

func TestCreateVessel_MissingName_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.CreateVessel(context.Background(), CreateVesselInput{
		CutoffDate: fixedNow.Add(24 * time.Hour),
		CreatedBy:  uuid.New(),
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestCreateVessel_MissingCutoff_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.CreateVessel(context.Background(), CreateVesselInput{
		Name:      "TORO",
		CreatedBy: uuid.New(),
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestCreateVessel_MissingCreatedBy_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.CreateVessel(context.Background(), CreateVesselInput{
		Name:       "TORO",
		CutoffDate: fixedNow.Add(24 * time.Hour),
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestCreateVessel_Valid_ReturnsVessel(t *testing.T) {
	ms := &mockStore{}
	svc := newSvc(ms)
	cutoff := fixedNow.Add(48 * time.Hour)
	v, err := svc.CreateVessel(context.Background(), CreateVesselInput{
		Name:       "TORO",
		CutoffDate: cutoff,
		CreatedBy:  uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Name != "TORO" {
		t.Errorf("name = %q, want TORO", v.Name)
	}
	if !v.CutoffDate.Equal(cutoff.UTC()) {
		t.Errorf("cutoff = %v, want %v", v.CutoffDate, cutoff.UTC())
	}
}

// ── UpdateVessel ──────────────────────────────────────────────────────────────

func TestUpdateVessel_MissingID_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.UpdateVessel(context.Background(), UpdateVesselInput{
		Name:       "X",
		CutoffDate: fixedNow.Add(time.Hour),
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestUpdateVessel_MissingName_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.UpdateVessel(context.Background(), UpdateVesselInput{
		ID:         uuid.New(),
		CutoffDate: fixedNow.Add(time.Hour),
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestUpdateVessel_MissingCutoff_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.UpdateVessel(context.Background(), UpdateVesselInput{
		ID:   uuid.New(),
		Name: "X",
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

// ── BookContainer ─────────────────────────────────────────────────────────────

func TestBookContainer_MissingVesselID_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.BookContainer(context.Background(), BookContainerInput{
		ContainerID: uuid.New(),
		BookedBy:    uuid.New(),
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestBookContainer_MissingContainerID_Returns400(t *testing.T) {
	svc := newSvc(&mockStore{})
	_, err := svc.BookContainer(context.Background(), BookContainerInput{
		VesselID: uuid.New(),
		BookedBy: uuid.New(),
	})
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestBookContainer_VesselNotFound_PropagatesError(t *testing.T) {
	notFound := domain.NewBizError(domain.ErrNotFound, "vessel not found")
	ms := &mockStore{vesselErr: notFound}
	svc := newSvc(ms)
	_, err := svc.BookContainer(context.Background(), BookContainerInput{
		VesselID:    uuid.New(),
		ContainerID: uuid.New(),
		BookedBy:    uuid.New(),
	})
	if !errors.Is(err, notFound) {
		t.Fatalf("want not-found error, got %v", err)
	}
}

func TestBookContainer_Valid_ReturnsBooking(t *testing.T) {
	vesselID := uuid.New()
	cutoff := fixedNow.Add(72 * time.Hour)
	ms := &mockStore{
		vessel: Vessel{ID: vesselID, Name: "VENUS", CutoffDate: cutoff},
	}
	svc := newSvc(ms)
	b, err := svc.BookContainer(context.Background(), BookContainerInput{
		VesselID:    vesselID,
		ContainerID: uuid.New(),
		BookedBy:    uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.VesselID != vesselID {
		t.Errorf("vessel_id = %v, want %v", b.VesselID, vesselID)
	}
	if !b.CutoffDate.Equal(cutoff) {
		t.Errorf("cutoff = %v, want %v", b.CutoffDate, cutoff)
	}
}

// ── UnbookContainer ───────────────────────────────────────────────────────────

func TestUnbookContainer_WrongVessel_Returns404(t *testing.T) {
	ms := &mockStore{
		booking: ShippingBooking{VesselID: uuid.New()}, // different vessel
	}
	svc := newSvc(ms)
	err := svc.UnbookContainer(context.Background(), uuid.New(), uuid.New())
	var biz *domain.BizError
	if !errors.As(err, &biz) || !errors.Is(biz.Unwrap(), domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUnbookContainer_Valid_Succeeds(t *testing.T) {
	vesselID := uuid.New()
	containerID := uuid.New()
	ms := &mockStore{
		booking: ShippingBooking{VesselID: vesselID, ContainerID: containerID},
	}
	svc := newSvc(ms)
	if err := svc.UnbookContainer(context.Background(), vesselID, containerID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── BookContainer + FreightCost ───────────────────────────────────────────────

func TestBookContainer_WithFreightCost_RoundTrips(t *testing.T) {
	vesselID := uuid.New()
	cutoff := fixedNow.Add(72 * time.Hour)
	ms := &mockStore{
		vessel: Vessel{ID: vesselID, Name: "VENUS", CutoffDate: cutoff},
	}
	svc := newSvc(ms)
	fc := &domain.Money{Amount: 150000, Currency: "VND"}
	b, err := svc.BookContainer(context.Background(), BookContainerInput{
		VesselID:    vesselID,
		ContainerID: uuid.New(),
		BookedBy:    uuid.New(),
		FreightCost: fc,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.FreightCost == nil {
		t.Fatal("want FreightCost set, got nil")
	}
	if b.FreightCost.Amount != 150000 || b.FreightCost.Currency != "VND" {
		t.Errorf("FreightCost = %+v, want {150000 VND}", b.FreightCost)
	}
}

func TestBookContainer_NoFreightCost_NilInResult(t *testing.T) {
	vesselID := uuid.New()
	cutoff := fixedNow.Add(72 * time.Hour)
	ms := &mockStore{
		vessel: Vessel{ID: vesselID, Name: "VENUS", CutoffDate: cutoff},
	}
	svc := newSvc(ms)
	b, err := svc.BookContainer(context.Background(), BookContainerInput{
		VesselID:    vesselID,
		ContainerID: uuid.New(),
		BookedBy:    uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.FreightCost != nil {
		t.Errorf("want FreightCost nil, got %+v", b.FreightCost)
	}
}
