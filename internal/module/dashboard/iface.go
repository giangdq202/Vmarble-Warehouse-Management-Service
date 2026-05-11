package dashboard

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type OverviewOutput struct {
	KPI            KPIOutput            `json:"kpi"`
	Charts         ChartsOutput         `json:"charts"`
	RecentActivity RecentActivityOutput `json:"recent_activity"`
}

type KPIOutput struct {
	Remnants         RemnantKPIOutput `json:"remnants"`
	UtilizationPct   float64          `json:"utilization_pct"`
	ActiveWorkOrders int              `json:"active_work_orders"`
	PendingCosting   int              `json:"pending_costing"`
}

type RemnantKPIOutput struct {
	Total     int `json:"total"`
	Available int `json:"available"`
	Allocated int `json:"allocated"`
	Consumed  int `json:"consumed"`
	Waste     int `json:"waste"`
}

type ChartsOutput struct {
	RemnantTrend7D []RemnantTrendPoint  `json:"remnant_trend_7d"`
	CostAllocation []CostAllocationItem `json:"cost_allocation"`
	MaterialUsage  []MaterialUsagePoint `json:"material_usage"`
}

type RemnantTrendPoint struct {
	Date      string `json:"date"`
	Available int    `json:"available"`
	Allocated int    `json:"allocated"`
	Waste     int    `json:"waste"`
}

type CostAllocationItem struct {
	SKUCode string `json:"sku_code"`
	Cost    int64  `json:"cost"`
}

type MaterialUsagePoint struct {
	Date      string  `json:"date"`
	Plywood   float64 `json:"PLYWOOD"`
	Metal     float64 `json:"METAL"`
	Accessory float64 `json:"ACCESSORY"`
}

type RecentActivityOutput struct {
	RecentCuts           []RecentCutItem                `json:"recent_cuts"`
	CompletedWorkOrders  []RecentWorkOrderItem          `json:"completed_work_orders"`
	CostingFinalizations []RecentCostingFinalizationItem `json:"costing_finalizations"`
}

type RecentCutItem struct {
	ID         uuid.UUID `json:"id"`
	WorkOrderID uuid.UUID `json:"work_order_id"`
	SKUID      uuid.UUID `json:"sku_id"`
	SKUCode    string    `json:"sku_code"`
	CreatedAt  time.Time `json:"created_at"`
}

type RecentWorkOrderItem struct {
	ID        uuid.UUID `json:"id"`
	SKUCode   string    `json:"sku_code"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type RecentCostingFinalizationItem struct {
	WorkOrderID uuid.UUID `json:"work_order_id"`
	SKUCode     string    `json:"sku_code"`
	TotalCost   int64     `json:"total_cost"`
	CreatedAt   time.Time `json:"created_at"`
}

// BoardStockSummaryItem aggregates whole board sheet counts and area per material.
type BoardStockSummaryItem struct {
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialName string    `json:"material_name"`
	Available    int       `json:"available"`
	Allocated    int       `json:"allocated"`
	AreaMM2      int64     `json:"area_mm2"`
}

// WIPStageRow is one entry in the per-stage WIP pipeline aggregation.
//
// Spec Pillar D (Real-time WIP) — surfaces how many work orders sit at each
// production stage and how many are at risk of missing the container ship
// date (deadline within the next 2 days).
type WIPStageRow struct {
	Status          string     `json:"status"`
	Count           int        `json:"count"`
	OldestStartedAt *time.Time `json:"oldest_started_at,omitempty"`
	AtRiskCount     int        `json:"at_risk_count"`
}

// WIPPipelineOutput is the response for GET /dashboard/wip-pipeline.
//
// Stages are returned in canonical state-machine order (PLANNED → IN_CUTTING
// → IN_PROCESSING → COMPLETED → COSTED). Stages with zero work orders are
// still included so the dashboard can render a stable five-column view.
type WIPPipelineOutput struct {
	Stages []WIPStageRow `json:"stages"`
}

type Service interface {
	GetOverview(ctx context.Context) (OverviewOutput, error)
	GetBoardStockSummary(ctx context.Context) ([]BoardStockSummaryItem, error)
	// GetWIPPipeline returns per-status work-order counts plus the oldest
	// work order's started timestamp and the count of "at risk" work orders
	// (production plan deadline < now + 2 days) at that stage.
	GetWIPPipeline(ctx context.Context) (WIPPipelineOutput, error)
}
