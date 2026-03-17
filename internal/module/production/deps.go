package production

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type PlanChecker interface {
	GetPlan(ctx context.Context, planID uuid.UUID) (PlanInfo, error)
}

type PlanInfo struct {
	ID     uuid.UUID
	Status domain.PlanStatus
}

type SKUChecker interface {
	GetSKU(ctx context.Context, skuID uuid.UUID) (SKUInfo, error)
}

type SKUInfo struct {
	ID            uuid.UUID
	RequiresMetal bool
}
