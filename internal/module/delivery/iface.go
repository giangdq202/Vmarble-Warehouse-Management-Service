// Package delivery owns containers — the unit by which finished goods leave
// the warehouse. A container scoops finished-goods quantities from one or more
// sales_order_lines, gets sealed once loading is complete, and is shipped.
// Sealing is the moment qty_shipped on the underlying SO lines actually moves.
package delivery

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// Container status values; mirror chk_container_status in 00051.
const (
	ContainerStatusOpen      = "OPEN"
	ContainerStatusLoading   = "LOADING"
	ContainerStatusSealed    = "SEALED"
	ContainerStatusShipped   = "SHIPPED"
	ContainerStatusCancelled = "CANCELLED"
)

// Container types with default capacity envelopes; client may override
// max_cbm / max_payload_kg in CreateContainerInput when a non-standard
// container is used. The defaults map to ISO 20GP / 40GP / 40HC.
const (
	ContainerType20GP = "20GP"
	ContainerType40GP = "40GP"
	ContainerType40HC = "40HC"
)

type Container struct {
	ID            uuid.UUID  `json:"id"`
	Code          string     `json:"code"`
	ContainerType string     `json:"container_type"`
	MaxCBM        float64    `json:"max_cbm"`
	MaxPayloadKG  float64    `json:"max_payload_kg"`
	Status        string     `json:"status"`
	SealedAt      *time.Time `json:"sealed_at,omitempty"`
	SealedBy      *uuid.UUID `json:"sealed_by,omitempty"`
	Note          string     `json:"note,omitempty"`
	CreatedBy     uuid.UUID  `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`

	// Computed projections — populated by GetContainer; List does not hydrate
	// these to keep the page query a single round-trip.
	Lines       []ContainerLine `json:"lines,omitempty"`
	UsedCBM     float64         `json:"used_cbm,omitempty"`
	UsedWeight  float64         `json:"used_weight_kg,omitempty"`
	FillPctCBM  float64         `json:"fill_pct_cbm,omitempty"`
	FillPctMass float64         `json:"fill_pct_mass,omitempty"`
}

type ContainerLine struct {
	ID               uuid.UUID `json:"id"`
	ContainerID      uuid.UUID `json:"container_id"`
	SKUID            uuid.UUID `json:"sku_id"`
	SKUCode          string    `json:"sku_code,omitempty"`
	SKUName          string    `json:"sku_name,omitempty"`
	Qty              int       `json:"qty"`
	SalesOrderLineID uuid.UUID `json:"sales_order_line_id"`
	CBMTotal         float64   `json:"cbm_total"`
	WeightKGTotal    float64   `json:"weight_kg_total"`
	AddedBy          uuid.UUID `json:"added_by"`
	AddedAt          time.Time `json:"added_at"`
}

type ContainerStatusLogEntry struct {
	ID          uuid.UUID `json:"id"`
	ContainerID uuid.UUID `json:"container_id"`
	FromStatus  string    `json:"from_status,omitempty"`
	ToStatus    string    `json:"to_status"`
	ActorID     uuid.UUID `json:"actor_id"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateContainerInput accepts max_cbm / max_payload_kg explicitly so the
// caller can override the defaults for a non-standard 20GP / 40GP / 40HC
// container. When zero, the service substitutes the default for the type.
type CreateContainerInput struct {
	ContainerType string  `json:"container_type"`
	MaxCBM        float64 `json:"max_cbm,omitempty"`
	MaxPayloadKG  float64 `json:"max_payload_kg,omitempty"`
	Note          string  `json:"note,omitempty"`
	CreatedBy     uuid.UUID `json:"-"`
}

// AddLineInput carries one finished-goods allocation onto a container.
//
// CBMTotal and WeightKGTotal are snapshotted at add time. Until issue #294
// adds height_mm / weight_kg / cbm to the catalog SKU, the client supplies
// these values from its own SKU registry. Once #294 lands, FE will compute
// `sku.cbm * qty` and `sku.weight_kg * qty` and pass them through unchanged
// — no service-side change required.
type AddLineInput struct {
	ContainerID      uuid.UUID `json:"-"`
	SKUID            uuid.UUID `json:"sku_id"`
	Qty              int       `json:"qty"`
	SalesOrderLineID uuid.UUID `json:"sales_order_line_id"`
	CBMTotal         float64   `json:"cbm_total"`
	WeightKGTotal    float64   `json:"weight_kg_total"`
	AddedBy          uuid.UUID `json:"-"`
}

// TransferLineInput moves part or all of a line from the source container
// (path :id) to the target. When Qty is zero the line is moved in full; when
// 0 < Qty < line.qty the source line is decremented and a new line is
// inserted at the target. Qty > line.qty returns ErrInvalidInput.
//
// CBMTotal / WeightKGTotal must be supplied for any partial transfer because
// the service cannot derive them: the original snapshot was for `line.qty`,
// not the new `Qty` slice. For a full transfer (Qty == 0) they are ignored
// and the source line's snapshot is reused unchanged.
type TransferLineInput struct {
	ContainerID       uuid.UUID `json:"-"`
	LineID            uuid.UUID `json:"line_id"`
	TargetContainerID uuid.UUID `json:"target_container_id"`
	Qty               int       `json:"qty,omitempty"`
	CBMTotal          float64   `json:"cbm_total,omitempty"`
	WeightKGTotal     float64   `json:"weight_kg_total,omitempty"`
	ActorID           uuid.UUID `json:"-"`
}

type TransferLineResult struct {
	SourceLine *ContainerLine `json:"source_line,omitempty"` // nil when the source line was fully consumed
	TargetLine ContainerLine  `json:"target_line"`
}

// SealInput carries the actor for the audit row. BR-D05: sealing flips the
// container to SEALED and atomically bumps qty_shipped on every underlying
// sales_order_line in a single transaction.
type SealInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Note        string    `json:"note,omitempty"`
}

// ReopenInput requires Reason — BR-D06 mandates an audit trail when an
// already-sealed container is reopened (admin only).
type ReopenInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Reason      string    `json:"reason"`
}

type ShipInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Note        string    `json:"note,omitempty"`
}

type CancelInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Reason      string    `json:"reason,omitempty"`
}

type ContainerListFilter struct {
	Status        string
	ContainerType string
}

type Service interface {
	CreateContainer(ctx context.Context, in CreateContainerInput) (Container, error)
	GetContainer(ctx context.Context, id uuid.UUID) (Container, error)
	ListContainers(ctx context.Context, p httpkit.PageParams, f ContainerListFilter) (httpkit.PagedResult[Container], error)

	AddLine(ctx context.Context, in AddLineInput) (ContainerLine, error)
	DeleteLine(ctx context.Context, containerID, lineID uuid.UUID, actorID uuid.UUID) error
	TransferLine(ctx context.Context, in TransferLineInput) (TransferLineResult, error)

	Seal(ctx context.Context, in SealInput) (Container, error)
	Reopen(ctx context.Context, in ReopenInput) (Container, error)
	Ship(ctx context.Context, in ShipInput) (Container, error)
	Cancel(ctx context.Context, in CancelInput) (Container, error)

	ListStatusLog(ctx context.Context, containerID uuid.UUID) ([]ContainerStatusLogEntry, error)
}

// DefaultCapacityForType returns the ISO defaults for a container type. When
// the caller omits max_cbm / max_payload_kg in CreateContainerInput, the
// service substitutes these. Returning (0,0,false) for an unknown type lets
// the service emit a clean ErrInvalidInput without an additional lookup.
func DefaultCapacityForType(t string) (cbm, payloadKG float64, ok bool) {
	switch t {
	case ContainerType20GP:
		return 33.2, 28000, true
	case ContainerType40GP:
		return 67.7, 26500, true
	case ContainerType40HC:
		return 76.4, 26500, true
	}
	return 0, 0, false
}
