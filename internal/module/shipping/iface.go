// Package shipping manages vessel schedules and container booking.
// The key domain invariant (BR-D08): a container cannot be sealed after
// its vessel's cutoff_date. cutoff_date is denormalized onto containers
// when a booking is created/updated so the delivery Seal() guard reads
// it from the container row without a join.
package shipping

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// Vessel represents a shipping vessel schedule entry.
type Vessel struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	Carrier           string     `json:"carrier,omitempty"`
	VoyageNumber      string     `json:"voyage_number,omitempty"`
	ETD               *time.Time `json:"etd,omitempty"`
	ETA               *time.Time `json:"eta,omitempty"`
	CutoffDate        time.Time  `json:"cutoff_date"`
	PortOfLoading     string     `json:"port_of_loading,omitempty"`
	PortOfDischarge   string     `json:"port_of_discharge,omitempty"`
	CreatedBy         uuid.UUID  `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ShippingBooking links one container to one vessel.
type ShippingBooking struct {
	ID          uuid.UUID  `json:"id"`
	VesselID    uuid.UUID  `json:"vessel_id"`
	ContainerID uuid.UUID  `json:"container_id"`
	BookingRef  string     `json:"booking_ref,omitempty"`
	BookedBy    uuid.UUID  `json:"booked_by"`
	BookedAt    time.Time  `json:"booked_at"`
	Note        string     `json:"note,omitempty"`

	// Hydrated fields — populated when reading back the booking.
	VesselName    string    `json:"vessel_name,omitempty"`
	CutoffDate    time.Time `json:"cutoff_date,omitempty"`
}

type CreateVesselInput struct {
	Name            string     `json:"name"`
	Carrier         string     `json:"carrier,omitempty"`
	VoyageNumber    string     `json:"voyage_number,omitempty"`
	ETD             *time.Time `json:"etd,omitempty"`
	ETA             *time.Time `json:"eta,omitempty"`
	CutoffDate      time.Time  `json:"cutoff_date"`
	PortOfLoading   string     `json:"port_of_loading,omitempty"`
	PortOfDischarge string     `json:"port_of_discharge,omitempty"`
	CreatedBy       uuid.UUID  `json:"-"`
}

type UpdateVesselInput struct {
	ID              uuid.UUID  `json:"-"`
	Name            string     `json:"name"`
	Carrier         string     `json:"carrier,omitempty"`
	VoyageNumber    string     `json:"voyage_number,omitempty"`
	ETD             *time.Time `json:"etd,omitempty"`
	ETA             *time.Time `json:"eta,omitempty"`
	CutoffDate      time.Time  `json:"cutoff_date"`
	PortOfLoading   string     `json:"port_of_loading,omitempty"`
	PortOfDischarge string     `json:"port_of_discharge,omitempty"`
}

type BookContainerInput struct {
	VesselID    uuid.UUID `json:"-"`
	ContainerID uuid.UUID `json:"container_id"`
	BookingRef  string    `json:"booking_ref,omitempty"`
	Note        string    `json:"note,omitempty"`
	BookedBy    uuid.UUID `json:"-"`
}

type VesselListFilter struct {
	// Optionally narrow to vessels whose cutoff_date is on or after this time.
	CutoffFrom *time.Time
}

type Service interface {
	CreateVessel(ctx context.Context, in CreateVesselInput) (Vessel, error)
	GetVessel(ctx context.Context, id uuid.UUID) (Vessel, error)
	ListVessels(ctx context.Context, p httpkit.PageParams, f VesselListFilter) (httpkit.PagedResult[Vessel], error)
	UpdateVessel(ctx context.Context, in UpdateVesselInput) (Vessel, error)
	DeleteVessel(ctx context.Context, id uuid.UUID) error

	// BookContainer assigns a container to this vessel. Overwrites any prior
	// booking for the container (upsert semantics: one container ↔ one vessel).
	// Denormalizes vessel.cutoff_date → containers.cutoff_date (BR-D08 source).
	BookContainer(ctx context.Context, in BookContainerInput) (ShippingBooking, error)

	// UnbookContainer removes the booking and clears containers.cutoff_date.
	UnbookContainer(ctx context.Context, vesselID, containerID uuid.UUID) error

	// GetContainerBooking returns the active booking for a container.
	GetContainerBooking(ctx context.Context, containerID uuid.UUID) (ShippingBooking, error)

	// ListContainerBookings returns all container bookings for a vessel.
	ListContainerBookings(ctx context.Context, vesselID uuid.UUID) ([]ShippingBooking, error)
}
