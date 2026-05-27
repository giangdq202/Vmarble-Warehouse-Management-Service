package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/module/authn"
	"github.com/vmarble/warehouse-management-service/internal/module/barcode"
	"github.com/vmarble/warehouse-management-service/internal/module/catalog"
	"github.com/vmarble/warehouse-management-service/internal/module/costing"
	"github.com/vmarble/warehouse-management-service/internal/module/dashboard"
	"github.com/vmarble/warehouse-management-service/internal/module/delivery"
	"github.com/vmarble/warehouse-management-service/internal/module/inventory"
	"github.com/vmarble/warehouse-management-service/internal/module/order"
	"github.com/vmarble/warehouse-management-service/internal/module/packing"
	"github.com/vmarble/warehouse-management-service/internal/module/planning"
	"github.com/vmarble/warehouse-management-service/internal/module/production"
	"github.com/vmarble/warehouse-management-service/internal/module/purchasing"
	"github.com/vmarble/warehouse-management-service/internal/module/reports"
	"github.com/vmarble/warehouse-management-service/internal/module/sales"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/config"
	"github.com/vmarble/warehouse-management-service/internal/platform/events"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
	"github.com/vmarble/warehouse-management-service/internal/platform/postgres"

	_ "github.com/vmarble/warehouse-management-service/docs"
)

// @title           VMARBLE Warehouse Management Service API
// @version         0.1.0
// @description     Backend API for warehouse & production management.
// @BasePath        /
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Enter: Bearer <token>
//
// @security BearerAuth
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	if err := postgres.Migrate(cfg.DatabaseURL, "migrations"); err != nil {
		slog.Error("run migrations", "err", err)
		os.Exit(1)
	}

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// ── Events infrastructure (SSE + PostgreSQL LISTEN/NOTIFY) ──────────────
	eventBroker := events.NewBroker()
	eventPublisher := events.NewPublisher(pool)
	go events.NewListener(cfg.DatabaseURL, eventBroker).Start(ctx)

	// ── Module stores ───────────────────────────────────────
	authnStore := authn.NewPGStore(pool)
	catalogStore := catalog.NewPGStore(pool)
	orderStore := order.NewPGStore(pool)
	planningStore := planning.NewPGStore(pool)
	inventoryStore := inventory.NewPGStore(pool)
	productionStore := production.NewPGStore(pool)
	costingStore := costing.NewPGStore(pool)
	dashboardStore := dashboard.NewPGStore(pool)
	barcodeStore := barcode.NewPGStore(pool)
	purchasingStore := purchasing.NewPGStore(pool)
	salesStore := sales.NewPGStore(pool)
	deliveryStore := delivery.NewPGStore(pool)
	packingStore := packing.NewPGStore(pool)

	// ── Module services ─────────────────────────────────────
	authnSvc := authn.NewService(authnStore, cfg.AuthSecret)
	catalogSvc := catalog.NewService(catalogStore)
	orderSvc := order.NewService(orderStore)
	// planningWOCanceller is wired after productionSvc is constructed (cycle
	// avoidance, same pattern as woAdvance/costingChecker). It powers the
	// APPROVED → CANCELED cascade introduced in #249.
	planningWOCanceller := &planningWOCancellerAdapter{}
	planningSvc := planning.NewServiceWithDeps(planningStore, planningWOCanceller)
	// woAdvanceAdapter is wired after productionSvc is constructed to avoid a
	// construction-time cycle (inventory → production → inventory).
	// costingChecker is similarly wired after costingSvc is constructed to avoid
	// production → costing → production cycle.
	woAdvance := &woAdvanceAdapter{}
	costingChecker := &costingCheckerAdapter{}
	barcodeGen := &cutBarcodeAdapter{planSvc: planningSvc}
	inventorySvc := inventory.NewServiceFull(
		inventoryStore,
		woAdvance,
		barcodeGen,
		eventPublisher,
		cfg.RemnantOverflowThresholdPct,
	)

	productionSvc := production.NewServiceFull(
		productionStore,
		&planAdapter{svc: planningSvc},
		&skuAdapter{svc: catalogSvc},
		&userAdapter{svc: authnSvc},
		&sheetAssignAdapter{svc: inventorySvc},
		costingChecker,
		eventPublisher,
		&remnantAdvisorAdapter{svc: inventorySvc},
		&stockCheckerAdapter{svc: inventorySvc},
		&bomReaderAdapter{svc: catalogSvc},
	)
	barcodeSvc := barcode.NewService(
		barcodeStore,
		&barcodeWOGatewayAdapter{svc: productionSvc},
		&barcodeUserLookupAdapter{svc: authnSvc},
		eventPublisher,
	)

	// Wire production into the advance adapter now that it exists.
	woAdvance.svc = productionSvc
	planningWOCanceller.svc = productionSvc
	barcodeGen.skuSvc = catalogSvc
	barcodeGen.woSvc = productionSvc
	barcodeGen.barcodeSvc = barcodeSvc

	costingSvc := costing.NewServiceFull(
		costingStore,
		&woAdapter{svc: productionSvc},
		&cuttingAdapter{pool: pool},
		&consumptionAdapter{pool: pool},
		&laborDataAdapter{svc: productionSvc},
		eventPublisher,
		&costingAuditAdapter{pool: pool},
	)
	// Wire costing into the checker adapter now that it exists.
	costingChecker.svc = costingSvc
	dashboardSvc := dashboard.NewService(dashboardStore)
	purchasingSvc := purchasing.NewService(
		purchasingStore,
		&purchasingMaterialAdapter{svc: catalogSvc},
		&purchasingStockAdapter{svc: inventorySvc},
	)

	reportsSvc := reports.NewService(
		&reportsCostingAdapter{pool: pool},
		&reportsPOAdapter{pool: pool},
		&reportsSKUAdapter{pool: pool},
		&reportsWOAdapter{pool: pool},
		&reportsWasteAdapter{svc: costingSvc},
	)

	// Sales depends on catalog (SKU existence) + planning + production (split-to-plan).
	// Constructed last so the splitter adapter can reference both planningSvc
	// and productionSvc directly without the post-wire trampoline pattern used
	// for the cyclical adapters above.
	salesSvc := sales.NewServiceWithAudit(
		salesStore,
		&salesSKUAdapter{svc: catalogSvc},
		&salesProductionSplitterAdapter{planSvc: planningSvc, woSvc: productionSvc},
		&salesMappingAuditAdapter{pool: pool},
	)

	// Delivery depends on catalog (SKU existence) + sales (SO line lookup +
	// the cross-module Tx hook for shipment recording at seal time). The
	// salesShipmentAdapter / deliverySOLineAdapter live below; deliverySKUAdapter
	// trims catalog.SKU down to the slim view delivery cares about.
	deliverySvc := delivery.NewService(
		deliveryStore,
		&deliverySKUAdapter{svc: catalogSvc},
		&deliverySOLineAdapter{svc: salesSvc},
		&deliveryShipmentAdapter{svc: salesSvc},
		cfg.ContainerCBMOverheadPct,
	)

	// Packing owns the FG pool. Constructed AFTER delivery so its container
	// suggestions / line-removal hook can reference deliverySvc directly.
	// Two cycle breaks below: production -> packing (FG hook on COMPLETED)
	// and delivery -> packing (FG track on AddLine/DeleteLine/Seal) — both
	// wired post-construction via setters because packing depends on those
	// modules in the other direction.
	packingSvc := packing.NewService(
		packingStore,
		&packingBarcodeIssuerAdapter{svc: barcodeSvc},
		&packingBarcodeResolverAdapter{svc: barcodeSvc},
		&packingWOGatewayAdapter{svc: productionSvc},
		&packingContainerSuggesterAdapter{svc: deliverySvc},
		&packingContainerLineRemoverAdapter{svc: deliverySvc},
		&packingDefectNotifierAdapter{publisher: eventPublisher},
	)

	// Wire the FG hooks now that all three services exist.
	if hooked, ok := productionSvc.(interface {
		SetFinishedGoodsHook(production.FinishedGoodsHook)
	}); ok {
		hooked.SetFinishedGoodsHook(&productionFGHookAdapter{svc: packingSvc})
	}
	if hooked, ok := deliverySvc.(interface {
		SetFGTracker(delivery.FGTracker)
	}); ok {
		hooked.SetFGTracker(&deliveryFGTrackerAdapter{svc: packingSvc})
	}
	// Loading-plan parser (#301) needs to translate customer-facing SKU codes
	// via sales.GetCustomerSKUMapping; audit hook records upload + approve.
	if hooked, ok := deliverySvc.(interface {
		SetCustomerSKUResolver(delivery.CustomerSKUResolver)
	}); ok {
		hooked.SetCustomerSKUResolver(&deliveryCustomerSKUAdapter{svc: salesSvc})
	}
	if hooked, ok := deliverySvc.(interface {
		SetLoadingPlanAuditor(delivery.LoadingPlanAuditLogger)
	}); ok {
		hooked.SetLoadingPlanAuditor(&deliveryLoadingPlanAuditAdapter{pool: pool})
	}

	// ── Background: auto-release expired remnant allocations ─────────────────
	// Ticks every cfg.RemnantAllocCheckInterval. Remnants that have been
	// ALLOCATED for longer than cfg.RemnantAllocTimeout without being consumed
	// are reset to AVAILABLE so they can be reassigned to other work orders.
	go func() {
		ticker := time.NewTicker(cfg.RemnantAllocCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				threshold := time.Now().UTC().Add(-cfg.RemnantAllocTimeout)
				released, err := inventorySvc.ReleaseExpiredAllocations(ctx, threshold)
				if err != nil {
					slog.Warn("remnant auto-release failed", "err", err)
					continue
				}
				if released > 0 {
					slog.Info("remnant auto-release: returned allocations to AVAILABLE",
						"count", released, "older_than", cfg.RemnantAllocTimeout)
				}
			}
		}
	}()

	// ── Gin router ──────────────────────────────────────────
	r := httpkit.NewRouter(pool)

	// Swagger UI: /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ── Public routes (no auth required) ───────────────────
	public := r.Group("/api/auth")
	authn.NewHandler(authnSvc).Register(public)

	// ── Protected routes ────────────────────────────────────
	api := r.Group("/api/v1")
	api.Use(auth.Middleware(cfg.AuthSecret))

	authnHandler := authn.NewHandler(authnSvc)
	authnHandler.RegisterProtected(api.Group("/users"))
	authnHandler.RegisterAdmin(api.Group("/admin"))

	catalog.NewHandler(catalogSvc).Register(api)
	order.NewHandler(orderSvc).Register(api)
	planning.NewHandler(planningSvc).Register(api)
	inventory.NewHandler(inventorySvc).Register(api)
	production.NewHandler(productionSvc).Register(api)
	costing.NewHandler(costingSvc).Register(api)
	dashboard.NewHandler(dashboardSvc).Register(api)
	barcode.NewHandler(barcodeSvc).Register(api)
	events.NewHandler(eventBroker).Register(api)
	purchasing.NewHandler(purchasingSvc).Register(api)
	reports.NewHandler(reportsSvc).Register(api)
	sales.NewHandler(salesSvc).Register(api)
	delivery.NewHandler(deliverySvc).Register(api)
	packing.NewHandler(packingSvc).Register(api)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}

// ── Adapters ────────────────────────────────────────────────

type planAdapter struct {
	svc planning.Service
}

func (a *planAdapter) GetPlan(ctx context.Context, planID uuid.UUID) (production.PlanInfo, error) {
	p, err := a.svc.GetPlan(ctx, planID)
	if err != nil {
		return production.PlanInfo{}, err
	}
	skuIDs := make([]uuid.UUID, len(p.Items))
	for i, item := range p.Items {
		skuIDs[i] = item.SKUID
	}
	return production.PlanInfo{ID: p.ID, Status: p.Status, SKUIDs: skuIDs}, nil
}

type skuAdapter struct {
	svc catalog.Service
}

func (a *skuAdapter) GetSKU(ctx context.Context, skuID uuid.UUID) (production.SKUInfo, error) {
	s, err := a.svc.GetSKU(ctx, skuID)
	if err != nil {
		return production.SKUInfo{}, err
	}
	return production.SKUInfo{
		ID:            s.ID,
		RequiresMetal: s.RequiresMetal,
		Dimensions:    s.Dimensions,
	}, nil
}

type userAdapter struct {
	svc authn.Service
}

func (a *userAdapter) GetUser(ctx context.Context, userID uuid.UUID) (production.UserInfo, error) {
	u, err := a.svc.GetUser(ctx, userID)
	if err != nil {
		return production.UserInfo{}, err
	}
	return production.UserInfo{ID: u.ID, Role: u.Role, IsActive: u.IsActive}, nil
}

type woAdapter struct {
	svc production.Service
}

func (a *woAdapter) GetWorkOrder(ctx context.Context, woID uuid.UUID) (costing.WOInfo, error) {
	wo, err := a.svc.GetWorkOrder(ctx, woID)
	if err != nil {
		return costing.WOInfo{}, err
	}
	return costing.WOInfo{ID: wo.ID, SKUID: wo.SKUID, Status: wo.Status}, nil
}

type sheetAssignAdapter struct {
	svc inventory.Service
}

func (a *sheetAssignAdapter) PreAssignSheet(ctx context.Context, in production.PreAssignSheetRequest) error {
	return a.svc.PreAssignSheet(ctx, inventory.PreAssignSheetInput{
		SheetID:        in.SheetID,
		WorkOrderID:    in.WorkOrderID,
		BypassOverflow: in.BypassOverflow,
		ActorID:        in.ActorID,
		Reason:         in.Reason,
	})
}

// remnantAdvisorAdapter implements production.RemnantAdvisor.
// Bridges production → inventory.SuggestRemnants + LogRemnantBypass for BR-K05.
type remnantAdvisorAdapter struct {
	svc inventory.Service
}

func (a *remnantAdvisorAdapter) SuggestRemnants(ctx context.Context, requiredDim domain.Dimension) ([]production.RemnantSuggestionRef, error) {
	suggestions, err := a.svc.SuggestRemnants(ctx, inventory.SuggestRemnantsInput{
		RequiredDimension: requiredDim,
	})
	if err != nil {
		return nil, err
	}
	out := make([]production.RemnantSuggestionRef, len(suggestions))
	for i, s := range suggestions {
		out[i] = production.RemnantSuggestionRef{RemnantID: s.Remnant.ID}
	}
	return out, nil
}

func (a *remnantAdvisorAdapter) LogRemnantBypass(ctx context.Context, in production.LogRemnantBypassRequest) error {
	return a.svc.LogRemnantBypass(ctx, inventory.LogRemnantBypassInput{
		WorkOrderID:         in.WorkOrderID,
		ActorID:             in.ActorID,
		SuggestedRemnantIDs: in.SuggestedRemnantIDs,
		Reason:              in.Reason,
	})
}

// stockCheckerAdapter implements production.StockChecker.
// Bridges production → inventory.CountAvailableSheetsByMaterial for BR-K01.
type stockCheckerAdapter struct {
	svc inventory.Service
}

func (a *stockCheckerAdapter) CountAvailableSheetsByMaterial(ctx context.Context, materialID uuid.UUID) (int, error) {
	return a.svc.CountAvailableSheetsByMaterial(ctx, materialID)
}

// bomReaderAdapter implements production.BOMReader. Resolves the SKU's BOM
// (DEFAULT variant or legacy) and returns only the SHEET-type material rows
// (catalog.MaterialTypePlywood) — auxiliaries (glue, metal) do not participate
// in the aggregate sheet stock check.
type bomReaderAdapter struct {
	svc catalog.Service
}

func (a *bomReaderAdapter) GetSheetMaterials(ctx context.Context, skuID uuid.UUID) ([]production.SheetRequirement, error) {
	bom, err := a.svc.GetBOMForVariant(ctx, skuID, "")
	if err != nil {
		return nil, err
	}
	out := make([]production.SheetRequirement, 0, len(bom.Components))
	for _, c := range bom.Components {
		if c.MaterialType == catalog.MaterialTypePlywood {
			out = append(out, production.SheetRequirement{MaterialID: c.MaterialID})
		}
	}
	return out, nil
}

// woAdvanceAdapter implements inventory.WorkOrderAdvancer.
// The svc field is set after productionSvc is constructed to break the
// inventory → production → inventory construction cycle.
type woAdvanceAdapter struct {
	svc production.Service
}

func (a *woAdvanceAdapter) AdvanceStatus(ctx context.Context, woID uuid.UUID, in inventory.AdvanceWOInput) error {
	return a.svc.AdvanceStatus(ctx, woID, production.AdvanceStatusInput{To: in.To})
}

type cutBarcodeAdapter struct {
	woSvc      production.Service
	skuSvc     catalog.Service
	planSvc    planning.Service
	barcodeSvc barcode.Service
}

func (a *cutBarcodeAdapter) GenerateForCut(ctx context.Context, in inventory.BarcodeForCutInput) (inventory.BarcodeForCutOutput, error) {
	if a.woSvc == nil || a.skuSvc == nil || a.planSvc == nil || a.barcodeSvc == nil {
		return inventory.BarcodeForCutOutput{}, nil
	}

	wo, err := a.woSvc.GetWorkOrder(ctx, in.WorkOrderID)
	if err != nil {
		return inventory.BarcodeForCutOutput{}, err
	}
	sku, err := a.skuSvc.GetSKU(ctx, wo.SKUID)
	if err != nil {
		return inventory.BarcodeForCutOutput{}, err
	}
	plan, err := a.planSvc.GetPlan(ctx, wo.PlanID)
	if err != nil {
		return inventory.BarcodeForCutOutput{}, err
	}
	// Cutting barcodes carry the originating PO id for traceability. SO-rooted
	// plans (Phase A pivot) leave it zero — the FG barcode flow (#291) is the
	// real customer-facing label and will use the SO id directly.
	var planPOID uuid.UUID
	if plan.POID != nil {
		planPOID = *plan.POID
	}

	wip, err := a.barcodeSvc.GenerateBarcode(ctx, barcode.GenerateBarcodeInput{
		WorkOrderID:      in.WorkOrderID,
		SKUID:            wo.SKUID,
		POID:             planPOID,
		ProductionPlanID: wo.PlanID,
		SKUCode:          sku.Code,
		SKUName:          sku.Name,
		Dimensions:       in.UsedDimension.String(),
		ProducedDate:     in.ProducedDate,
	})
	if err != nil {
		return inventory.BarcodeForCutOutput{}, err
	}

	out := inventory.BarcodeForCutOutput{WIPBarcodeID: &wip.ID}
	if in.RemnantDimension != nil {
		remnant, err := a.barcodeSvc.GenerateBarcode(ctx, barcode.GenerateBarcodeInput{
			WorkOrderID:      in.WorkOrderID,
			SKUID:            wo.SKUID,
			POID:             planPOID,
			ProductionPlanID: wo.PlanID,
			SKUCode:          sku.Code,
			SKUName:          sku.Name + " [REMNANT]",
			Dimensions:       in.RemnantDimension.String(),
			ProducedDate:     in.ProducedDate,
		})
		if err != nil {
			return out, err
		}
		out.RemnantBarcodeID = &remnant.ID
	}
	return out, nil
}

type barcodeWOGatewayAdapter struct {
	svc production.Service
}

func (a *barcodeWOGatewayAdapter) GetWorkOrder(ctx context.Context, woID uuid.UUID) (barcode.WorkOrderRef, error) {
	wo, err := a.svc.GetWorkOrder(ctx, woID)
	if err != nil {
		return barcode.WorkOrderRef{}, err
	}
	return barcode.WorkOrderRef{
		ID:      wo.ID,
		Status:  wo.Status,
		SKUCode: wo.SKUCode,
		SKUName: wo.SKUName,
	}, nil
}

func (a *barcodeWOGatewayAdapter) AdvanceStatus(ctx context.Context, woID uuid.UUID, to domain.WorkOrderStatus) error {
	return a.svc.AdvanceStatus(ctx, woID, production.AdvanceStatusInput{To: to})
}

type barcodeUserLookupAdapter struct {
	svc authn.Service
}

func (a *barcodeUserLookupAdapter) GetUser(ctx context.Context, userID uuid.UUID) (barcode.UserRef, error) {
	u, err := a.svc.GetUser(ctx, userID)
	if err != nil {
		return barcode.UserRef{}, err
	}
	return barcode.UserRef{ID: u.ID, Username: u.Username}, nil
}

type cuttingAdapter struct {
	pool *pgxpool.Pool
}

func (a *cuttingAdapter) GetCuttingDataForWO(ctx context.Context, woID uuid.UUID) ([]costing.CuttingData, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT bs.cost_amount, bs.cost_currency,
		        CAST(bs.length_mm AS bigint) * CAST(bs.width_mm AS bigint),
		        CAST(cr.used_length_mm AS bigint) * CAST(cr.used_width_mm AS bigint)
		 FROM cutting_records cr
		 JOIN board_sheets bs ON bs.id = cr.sheet_id
		 WHERE cr.work_order_id = $1`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []costing.CuttingData
	for rows.Next() {
		var cd costing.CuttingData
		if err := rows.Scan(&cd.SheetCost.Amount, &cd.SheetCost.Currency,
			&cd.SheetAreaMM2, &cd.UsedAreaMM2); err != nil {
			return nil, err
		}
		data = append(data, cd)
	}
	return data, rows.Err()
}

type consumptionAdapter struct {
	pool *pgxpool.Pool
}

func (a *consumptionAdapter) GetConsumptionCostForWO(ctx context.Context, woID uuid.UUID) (domain.Money, error) {
	var total int64
	err := a.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity::bigint), 0) FROM consumption_records WHERE work_order_id = $1`, woID).
		Scan(&total)
	if err != nil {
		return domain.Money{}, err
	}
	return domain.VND(total), nil
}

type purchasingMaterialAdapter struct {
	svc catalog.Service
}

func (a *purchasingMaterialAdapter) GetMaterial(ctx context.Context, materialID uuid.UUID) (purchasing.MaterialInfo, error) {
	m, err := a.svc.GetMaterial(ctx, materialID)
	if err != nil {
		return purchasing.MaterialInfo{}, err
	}
	return purchasing.MaterialInfo{ID: m.ID, Name: m.Name, Unit: m.Unit}, nil
}

type purchasingStockAdapter struct {
	svc inventory.Service
}

func (a *purchasingStockAdapter) ReceiveStock(ctx context.Context, in purchasing.ReceiveStockInput) (uuid.UUID, error) {
	lot, err := a.svc.ReceiveStock(ctx, inventory.ReceiveStockInput{
		MaterialID:   in.MaterialID,
		Dimensions:   domain.Dimension{LengthMM: in.LengthMM, WidthMM: in.WidthMM},
		CostPerSheet: in.UnitCost,
		Quantity:     in.Quantity,
		SupplierRef:  in.SupplierRef,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return lot.ID, nil
}

// costingCheckerAdapter implements production.CostingChecker.
// The svc field is set after costingSvc is constructed to break the
// production → costing → production construction cycle.
type costingCheckerAdapter struct {
	svc costing.Service
}

func (a *costingCheckerAdapter) HasCostingRecord(ctx context.Context, workOrderID uuid.UUID) (bool, error) {
	if a.svc == nil {
		return false, nil
	}
	return a.svc.HasCostingRecord(ctx, workOrderID)
}

func (a *costingCheckerAdapter) IsCostingFinalized(ctx context.Context, workOrderID uuid.UUID) (bool, error) {
	if a.svc == nil {
		return false, nil
	}
	return a.svc.IsCostingFinalized(ctx, workOrderID)
}

// planningWOCancellerAdapter bridges planning → production for the APPROVED
// → CANCELED cascade introduced in #249. The svc field is wired after
// productionSvc is constructed (production also depends on planning via
// planAdapter, so eager wiring would cycle).
type planningWOCancellerAdapter struct {
	svc production.Service
}

func (a *planningWOCancellerAdapter) ListStatusesByPlan(ctx context.Context, planID uuid.UUID) ([]domain.WorkOrderStatus, error) {
	return a.svc.ListStatusesByPlan(ctx, planID)
}

func (a *planningWOCancellerAdapter) CancelPlannedByPlan(ctx context.Context, planID uuid.UUID) (int64, error) {
	return a.svc.CancelPlannedByPlan(ctx, planID)
}

// laborDataAdapter implements costing.LaborDataReader by delegating to the
// production module's SumLaborCost. Wired in main.go after productionSvc is
// constructed.
type laborDataAdapter struct {
	svc production.Service
}

func (a *laborDataAdapter) GetLaborCostForWO(ctx context.Context, woID uuid.UUID) (domain.Money, error) {
	return a.svc.SumLaborCost(ctx, woID)
}

// ── Reports adapters (#257, refactored #273) ───────────────────────────────
//
// Each adapter projects pgxpool rows directly to reports.<Row> and yields
// them through a callback so the streaming export pipeline holds at most one
// row in memory regardless of dataset size. Reads happen through SQL with
// optional [from, to) bounds so the export endpoints filter by period
// without touching the source-module list APIs.

type reportsCostingAdapter struct{ pool *pgxpool.Pool }

func (a *reportsCostingAdapter) IterateCostingsInPeriod(ctx context.Context, p reports.Period, yield func(reports.CostingRow) error) error {
	q := `
		SELECT cr.work_order_id, s.code, s.name, cr.costing_type,
		       cr.material_cost_amount, cr.auxiliary_cost_amount, cr.labor_cost_amount, cr.total_cost_amount,
		       cr.finalized, cr.created_at
		FROM costing_records cr
		JOIN skus s ON s.id = cr.sku_id
		WHERE ($1::timestamptz IS NULL OR cr.created_at >= $1)
		  AND ($2::timestamptz IS NULL OR cr.created_at <  $2)
		ORDER BY cr.created_at DESC`
	rows, err := a.pool.Query(ctx, q, p.From, p.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r reports.CostingRow
		if err := rows.Scan(&r.WorkOrderID, &r.SKUCode, &r.SKUName, &r.CostingType,
			&r.MaterialCost, &r.AuxiliaryCost, &r.LaborCost, &r.TotalCost,
			&r.Finalized, &r.CreatedAt); err != nil {
			return err
		}
		if err := yield(r); err != nil {
			return err
		}
	}
	return rows.Err()
}

type reportsPOAdapter struct{ pool *pgxpool.Pool }

func (a *reportsPOAdapter) IteratePOsInPeriod(ctx context.Context, p reports.Period, yield func(reports.PORow) error) error {
	q := `
		SELECT po.code, COALESCE(po.supplier, ''), po.status, COALESCE(m.name, ''),
		       po.ordered_at, po.received_at,
		       COALESCE((
		           SELECT string_agg(
		                  i.quantity || '×' || i.length_mm || '×' || i.width_mm
		                  || ' @' || i.unit_cost_amount,
		                  '; ' ORDER BY i.id)
		           FROM material_purchase_order_items i WHERE i.po_id = po.id
		       ), ''),
		       COALESCE((
		           SELECT SUM(i.quantity::bigint * i.unit_cost_amount)
		           FROM material_purchase_order_items i WHERE i.po_id = po.id
		       ), 0),
		       po.created_at
		FROM material_purchase_orders po
		LEFT JOIN materials m ON m.id = po.material_id
		WHERE ($1::timestamptz IS NULL OR po.created_at >= $1)
		  AND ($2::timestamptz IS NULL OR po.created_at <  $2)
		ORDER BY po.created_at DESC`
	rows, err := a.pool.Query(ctx, q, p.From, p.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r reports.PORow
		if err := rows.Scan(&r.Code, &r.Supplier, &r.Status, &r.MaterialName,
			&r.OrderedAt, &r.ReceivedAt, &r.Items, &r.TotalCost, &r.CreatedAt); err != nil {
			return err
		}
		if err := yield(r); err != nil {
			return err
		}
	}
	return rows.Err()
}

type reportsSKUAdapter struct{ pool *pgxpool.Pool }

// IterateSKUs binds the streaming-cap as a SQL LIMIT so the catalog cannot
// silently grow past the bound — even if the source table briefly inflates,
// the DB-side cap keeps RAM bounded for both pgx and excelize.
func (a *reportsSKUAdapter) IterateSKUs(ctx context.Context, limit int, yield func(reports.SKURow) error) error {
	q := `
		SELECT s.code, s.name, s.length_mm, s.width_mm, s.requires_metal,
		       COALESCE(s.is_active, true),
		       COALESCE((
		           SELECT string_agg(m.name || ' × ' || bc.quantity_per_unit, '; ' ORDER BY m.name)
		           FROM bom_components bc JOIN materials m ON m.id = bc.material_id
		           WHERE bc.sku_id = s.id
		       ), '')
		FROM skus s
		ORDER BY s.code
		LIMIT $1`
	rows, err := a.pool.Query(ctx, q, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r reports.SKURow
		if err := rows.Scan(&r.Code, &r.Name, &r.LengthMM, &r.WidthMM,
			&r.RequiresMetal, &r.IsActive, &r.BOMSummary); err != nil {
			return err
		}
		if err := yield(r); err != nil {
			return err
		}
	}
	return rows.Err()
}

type reportsWOAdapter struct{ pool *pgxpool.Pool }

func (a *reportsWOAdapter) IterateWorkOrdersInPeriod(ctx context.Context, p reports.Period, yield func(reports.WORow) error) error {
	q := `
		SELECT wo.id::text, s.code, s.name, wo.quantity, wo.status,
		       wo.assigned_at, wo.created_at,
		       (SELECT cr.total_cost_amount FROM costing_records cr WHERE cr.work_order_id = wo.id LIMIT 1)
		FROM work_orders wo
		JOIN skus s ON s.id = wo.sku_id
		WHERE ($1::timestamptz IS NULL OR wo.created_at >= $1)
		  AND ($2::timestamptz IS NULL OR wo.created_at <  $2)
		ORDER BY wo.created_at DESC`
	rows, err := a.pool.Query(ctx, q, p.From, p.To)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r reports.WORow
		var totalCost *int64
		if err := rows.Scan(&r.ID, &r.SKUCode, &r.SKUName, &r.Quantity, &r.Status,
			&r.AssignedAt, &r.CreatedAt, &totalCost); err != nil {
			return err
		}
		r.TotalCost = totalCost
		if err := yield(r); err != nil {
			return err
		}
	}
	return rows.Err()
}

// reportsWasteAdapter delegates to costing.Service.ListWasteReport so the
// per-material aggregation logic stays single-sourced. The waste report is
// an aggregate (one row per material), so the result set is naturally
// bounded by the number of distinct materials — streaming is still useful
// to keep the call shape consistent with the other readers.
type reportsWasteAdapter struct{ svc costing.Service }

func (a *reportsWasteAdapter) IterateWasteInPeriod(ctx context.Context, p reports.Period, yield func(reports.WasteRow) error) error {
	rows, err := a.svc.ListWasteReport(ctx, costing.WasteReportFilter{From: p.From, To: p.To})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := yield(reports.WasteRow{
			MaterialName:   r.MaterialName,
			SheetsConsumed: r.SheetsConsumed,
			WasteAreaMM2:   r.WasteAreaMM2,
			AvgSheetCost:   r.AvgSheetCost.Amount,
			TotalWasteCost: r.TotalWasteCost.Amount,
		}); err != nil {
			return err
		}
	}
	return nil
}

// costingAuditAdapter implements costing.AuditLogger by writing directly to
// inventory_audit_log via pgxpool. Inlined here (instead of going through
// inventory.Service) so costing does not depend on the inventory module —
// the audit table is a shared cross-module ledger keyed by entity_type.
//
// COSTING_ADJUSTED rows carry entity_type="COSTING_ADJUSTMENT", entity_id =
// adjustment.id; the metadata JSON column holds per-axis deltas + the parent
// costing_record_id and work_order_id so accountants can reconstruct the
// change without joining.
type costingAuditAdapter struct{ pool *pgxpool.Pool }

func (a *costingAuditAdapter) LogCostingAdjustment(ctx context.Context, in costing.AuditCostingAdjustmentInput) error {
	meta, err := json.Marshal(map[string]any{
		"costing_record_id": in.CostingRecordID,
		"work_order_id":     in.WorkOrderID,
		"delta_material":    in.DeltaMaterial.Amount,
		"delta_auxiliary":   in.DeltaAuxiliary.Amount,
		"delta_labor":       in.DeltaLabor.Amount,
		"delta_total":       in.DeltaTotal.Amount,
		"currency":          in.DeltaTotal.Currency,
	})
	if err != nil {
		return err
	}
	_, err = a.pool.Exec(ctx,
		`INSERT INTO inventory_audit_log
		    (id, entity_type, entity_id, action, actor_id, reason, metadata, created_at)
		 VALUES (gen_random_uuid(), 'COSTING_ADJUSTMENT', $1, 'COSTING_ADJUSTED', $2, $3, $4, NOW())`,
		in.AdjustmentID, in.ActorID, in.Reason, meta,
	)
	return err
}

// salesMappingAuditAdapter implements sales.CustomerSKUMappingAuditLogger by
// writing directly to inventory_audit_log via pgxpool. Inlined here (instead of
// going through inventory.Service) so sales does not depend on the inventory
// module — the audit table is a shared cross-module ledger keyed by
// entity_type. Errors are swallowed by the service (best-effort): a transient
// audit-write failure must never roll back the mapping mutation itself.
//
// CSM_* rows carry entity_type="CUSTOMER_SKU_MAPPING". The composite PK
// (customer_id, customer_sku_code) is not a UUID, so entity_id is a fresh
// gen_random_uuid() and the real key lands in the metadata JSON alongside any
// previous_sku_id (UPDATEs that moved sku_id) or rows_imported (bulk).
type salesMappingAuditAdapter struct{ pool *pgxpool.Pool }

func (a *salesMappingAuditAdapter) LogCustomerSKUMapping(ctx context.Context, in sales.AuditCustomerSKUMappingInput) error {
	meta := map[string]any{
		"customer_id":       in.CustomerID,
		"customer_sku_code": in.CustomerSKUCode,
	}
	if in.SKUID != uuid.Nil {
		meta["sku_id"] = in.SKUID
	}
	if in.PreviousSKUID != nil {
		meta["previous_sku_id"] = *in.PreviousSKUID
	}
	if in.RowsImported > 0 {
		meta["rows_imported"] = in.RowsImported
	}
	if in.Notes != "" {
		meta["notes"] = in.Notes
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = a.pool.Exec(ctx,
		`INSERT INTO inventory_audit_log
		    (id, entity_type, entity_id, action, actor_id, reason, metadata, created_at)
		 VALUES (gen_random_uuid(), 'CUSTOMER_SKU_MAPPING', gen_random_uuid(), $1, $2, $3, $4, NOW())`,
		string(in.Action), in.ActorID, in.Notes, metaJSON,
	)
	return err
}

// salesSKUAdapter implements sales.SKUChecker by delegating to catalog.GetSKU.
// Sales only needs existence + a slim projection — not pricing, not BOM —
// so the adapter trims the catalog SKU down to the three fields the sales
// service treats as authoritative.
type salesSKUAdapter struct {
	svc catalog.Service
}

func (a *salesSKUAdapter) GetSKU(ctx context.Context, skuID uuid.UUID) (sales.SKUInfo, error) {
	s, err := a.svc.GetSKU(ctx, skuID)
	if err != nil {
		return sales.SKUInfo{}, err
	}
	return sales.SKUInfo{ID: s.ID, Code: s.Code, Name: s.Name}, nil
}

// salesProductionSplitterAdapter implements sales.ProductionSplitter. It
// composes planning.CreatePlan + production.CreateWorkOrder per item to
// realize a SplitToPlan request from sales.
//
// The plan is created with SOID set (no PO) — supported since the planning
// refactor that landed alongside #289. Each WO carries SalesOrderLineID so
// the SO → Plan → WO lineage survives end-to-end.
//
// Atomicity caveat (mirrored in sales/deps.go): plan + WOs are inserted
// across two cross-module calls, not a single pool-level transaction. If a
// WO insert fails after the plan is created, the plan is left as DRAFT and
// the orphan is flagged via the returned error. Sales' own qty_planned
// mutation runs in a third, separate transaction; see service.go SplitToPlan
// for the full failure mode contract.
type salesProductionSplitterAdapter struct {
	planSvc planning.Service
	woSvc   production.Service
}

func (a *salesProductionSplitterAdapter) CreatePlanWithWOs(ctx context.Context, in sales.CreatePlanWithWOsRequest) (sales.CreatePlanWithWOsResult, error) {
	planItems := make([]planning.PlanItemInput, len(in.Items))
	for i, it := range in.Items {
		planItems[i] = planning.PlanItemInput{SKUID: it.SKUID, Quantity: it.Quantity}
	}
	soID := in.SalesOrderID
	plan, err := a.planSvc.CreatePlan(ctx, planning.CreatePlanInput{
		SOID:     &soID,
		Items:    planItems,
		Deadline: in.Deadline,
	})
	if err != nil {
		return sales.CreatePlanWithWOsResult{}, err
	}

	woIDs := make([]uuid.UUID, 0, len(in.Items))
	for _, it := range in.Items {
		soLineID := it.SOLineID
		actorID := in.ActorID
		wo, err := a.woSvc.CreateWorkOrder(ctx, production.CreateWOInput{
			PlanID:           plan.ID,
			SKUID:            it.SKUID,
			Quantity:         it.Quantity,
			SalesOrderLineID: &soLineID,
			CallerID:         &actorID,
		})
		if err != nil {
			return sales.CreatePlanWithWOsResult{}, err
		}
		woIDs = append(woIDs, wo.ID)
	}
	return sales.CreatePlanWithWOsResult{
		PlanID:       plan.ID,
		PlanCode:     plan.Code,
		WorkOrderIDs: woIDs,
	}, nil
}

// deliverySKUAdapter implements delivery.SKUChecker by trimming catalog.SKU
// down to the slim view delivery.AddLine validates against (existence + the
// SKU code/name used in audit messages).
type deliverySKUAdapter struct {
	svc catalog.Service
}

func (a *deliverySKUAdapter) GetSKU(ctx context.Context, skuID uuid.UUID) (delivery.SKUInfo, error) {
	s, err := a.svc.GetSKU(ctx, skuID)
	if err != nil {
		return delivery.SKUInfo{}, err
	}
	return delivery.SKUInfo{ID: s.ID, Code: s.Code, Name: s.Name}, nil
}

// deliverySOLineAdapter implements delivery.SOLineChecker by joining
// sales.GetSOLine's two-tuple result into the slim projection delivery
// uses for AddLine validation (matching SKU + SO status + qty math).
type deliverySOLineAdapter struct {
	svc sales.Service
}

func (a *deliverySOLineAdapter) GetSOLine(ctx context.Context, soLineID uuid.UUID) (delivery.SOLineInfo, error) {
	line, so, err := a.svc.GetSOLine(ctx, soLineID)
	if err != nil {
		return delivery.SOLineInfo{}, err
	}
	return delivery.SOLineInfo{
		ID:         line.ID,
		SOID:       so.ID,
		SOStatus:   so.Status,
		SKUID:      line.SKUID,
		QtyPlanned: line.QtyPlanned,
		QtyShipped: line.QtyShipped,
	}, nil
}

// deliveryShipmentAdapter implements delivery.ShipmentRecorder. The Tx the
// caller hands in is delivery's own transaction — sales runs its qty_shipped
// bump + SO status recompute inside it so seal is atomic. This is the
// deliberate cross-module Tx leak documented in delivery/deps.go.
type deliveryShipmentAdapter struct {
	svc sales.Service
}

func (a *deliveryShipmentAdapter) RecordShipmentTx(ctx context.Context, tx pgx.Tx, items []delivery.ShipmentItem) error {
	out := make([]sales.ShipmentItemInput, len(items))
	for i, it := range items {
		out[i] = sales.ShipmentItemInput{SOLineID: it.SOLineID, Qty: it.Qty}
	}
	return a.svc.RecordShipmentTx(ctx, tx, out)
}

// ── Packing adapters (#291) ────────────────────────────────────────────────

// packingBarcodeIssuerAdapter implements packing.BarcodeIssuer by delegating
// to the existing barcode module. The packing module needs to mint exactly
// one barcode per FG row when WO advances to COMPLETED.
type packingBarcodeIssuerAdapter struct {
	svc barcode.Service
}

func (a *packingBarcodeIssuerAdapter) GenerateBarcode(ctx context.Context, in packing.BarcodeIssueInput) (packing.BarcodeRef, error) {
	bc, err := a.svc.GenerateBarcode(ctx, barcode.GenerateBarcodeInput{
		WorkOrderID:      in.WorkOrderID,
		SKUID:            in.SKUID,
		POID:             in.POID,
		ProductionPlanID: in.ProductionPlanID,
		SKUCode:          in.SKUCode,
		SKUName:          in.SKUName,
		Dimensions:       in.Dimensions,
		ProducedDate:     in.ProducedDate,
	})
	if err != nil {
		return packing.BarcodeRef{}, err
	}
	return packing.BarcodeRef{ID: bc.ID}, nil
}

// packingBarcodeResolverAdapter implements packing.BarcodeResolver via
// barcode.LookupBarcode. Used at scan time to validate that the scanned
// barcode exists before consulting fg_pool.
type packingBarcodeResolverAdapter struct {
	svc barcode.Service
}

func (a *packingBarcodeResolverAdapter) LookupBarcode(ctx context.Context, barcodeID uuid.UUID) (packing.BarcodeLookup, error) {
	bc, err := a.svc.LookupBarcode(ctx, barcodeID)
	if err != nil {
		return packing.BarcodeLookup{}, err
	}
	return packing.BarcodeLookup{
		ID:          bc.ID,
		WorkOrderID: bc.WorkOrderID,
		SKUID:       bc.SKUID,
	}, nil
}

// packingWOGatewayAdapter implements packing.WorkOrderGateway. ScanBarcode
// uses it to enforce BR-PK01 (scan only valid after WO COMPLETED).
type packingWOGatewayAdapter struct {
	svc production.Service
}

func (a *packingWOGatewayAdapter) GetWorkOrderStatus(ctx context.Context, woID uuid.UUID) (packing.WorkOrderStatusInfo, error) {
	wo, err := a.svc.GetWorkOrder(ctx, woID)
	if err != nil {
		return packing.WorkOrderStatusInfo{}, err
	}
	return packing.WorkOrderStatusInfo{ID: wo.ID, Status: string(wo.Status)}, nil
}

// packingContainerSuggesterAdapter projects delivery.GetContainer (per-SO-line
// view) onto the slim suggestion shape packing surfaces at scan time. The
// kiosk receives OPEN/LOADING containers carrying the same SO line, ordered
// by fill_pct ascending so the operator picks the most-empty container first.
type packingContainerSuggesterAdapter struct {
	svc delivery.Service
}

func (a *packingContainerSuggesterAdapter) SuggestForSOLine(ctx context.Context, soLineID uuid.UUID) ([]packing.ContainerSuggestion, error) {
	// Page through containers carrying this SO line via the standard list
	// endpoint. For v1 we rely on the page filter — when the volume of OPEN
	// containers grows, a dedicated SuggestForSOLine query in delivery.pgstore
	// can replace this without touching the packing surface.
	res, err := a.svc.ListContainers(ctx, httpkit.PageParams{Page: 1, Limit: 50}, delivery.ContainerListFilter{
		Status: delivery.ContainerStatusLoading,
	})
	if err != nil {
		return nil, err
	}
	out := make([]packing.ContainerSuggestion, 0, len(res.Items))
	for _, c := range res.Items {
		// Hydrate fill metrics from the Get payload — list response keeps
		// the page query a single round-trip and skips per-row aggregation.
		full, err := a.svc.GetContainer(ctx, c.ID)
		if err != nil {
			continue
		}
		hasLine := false
		for _, l := range full.Lines {
			if l.SalesOrderLineID == soLineID {
				hasLine = true
				break
			}
		}
		if !hasLine {
			continue
		}
		out = append(out, packing.ContainerSuggestion{
			ContainerID:   full.ID,
			Code:          full.Code,
			Status:        full.Status,
			FillPctCBM:    full.FillPctCBM,
			FillPctMass:   full.FillPctMass,
			ContainerType: full.ContainerType,
		})
	}
	return out, nil
}

// packingContainerLineRemoverAdapter implements packing.ContainerLineRemover
// by walking the FG → container_line chain back through delivery.DeleteLine.
// Needed by ReportDefect when a RESERVED FG is flagged: the line stops
// counting toward container qty/cbm/weight before the FG flips to DEFECT.
type packingContainerLineRemoverAdapter struct {
	svc delivery.Service
}

func (a *packingContainerLineRemoverAdapter) DeleteLineForDefect(ctx context.Context, containerLineID uuid.UUID, actorID uuid.UUID) error {
	// Resolve the container_id from the line via a lightweight list scan;
	// the active-set is small (OPEN/LOADING containers only). For Sprint 8
	// when the warehouse scales, replace with a dedicated lookup.
	res, err := a.svc.ListContainers(ctx, httpkit.PageParams{Page: 1, Limit: 200}, delivery.ContainerListFilter{})
	if err != nil {
		return err
	}
	for _, c := range res.Items {
		if c.Status != delivery.ContainerStatusOpen && c.Status != delivery.ContainerStatusLoading {
			continue
		}
		full, err := a.svc.GetContainer(ctx, c.ID)
		if err != nil {
			continue
		}
		for _, l := range full.Lines {
			if l.ID == containerLineID {
				return a.svc.DeleteLine(ctx, c.ID, containerLineID, actorID)
			}
		}
	}
	return nil // line not found in active containers — assume already removed
}

// packingDefectNotifierAdapter implements packing.DefectNotifier on top of
// the platform events publisher.
type packingDefectNotifierAdapter struct {
	publisher *events.Publisher
}

func (a *packingDefectNotifierAdapter) NotifyFGDefect(ctx context.Context, fgID uuid.UUID, skuCode, reason string) error {
	return a.publisher.NotifyFGDefect(ctx, fgID.String(), skuCode, reason)
}

func (a *packingDefectNotifierAdapter) NotifyFGDefectResolved(ctx context.Context, fgID uuid.UUID, resolution string) error {
	return a.publisher.NotifyFGDefectResolved(ctx, fgID.String(), resolution)
}

// productionFGHookAdapter implements production.FinishedGoodsHook by
// delegating to packing.CreateFromCompletedWO. Wired post-construction
// because the production -> packing path otherwise cycles with the
// packing -> production WorkOrderGateway.
type productionFGHookAdapter struct {
	svc packing.Service
}

func (a *productionFGHookAdapter) OnWOCompleted(ctx context.Context, evt production.WOCompletedEvent) error {
	_, err := a.svc.CreateFromCompletedWO(ctx, packing.CreateFromCompletedWOInput{
		WorkOrderID:      evt.WorkOrderID,
		SKUID:            evt.SKUID,
		SKUCode:          evt.SKUCode,
		SKUName:          evt.SKUName,
		Dimensions:       evt.Dimensions,
		Quantity:         evt.Quantity,
		SalesOrderLineID: evt.SalesOrderLineID,
		ProductionPlanID: evt.ProductionPlanID,
		QCPassedBy:       evt.QCPassedBy,
		ProducedDate:     time.Now(),
	})
	return err
}

// deliveryFGTrackerAdapter implements delivery.FGTracker by delegating to the
// packing module. AddLine / DeleteLine / Seal each call into packing as a
// best-effort hook — packing owns the FG pool, delivery owns the container
// state, the two stay loosely coupled so neither blocks the other on
// transient failure.
type deliveryFGTrackerAdapter struct {
	svc packing.Service
}

func (a *deliveryFGTrackerAdapter) ReserveOnAdd(ctx context.Context, in delivery.FGReserveRequest) (int, error) {
	return a.svc.ReserveOnContainerAdd(ctx, packing.ReserveInput{
		SKUID:            in.SKUID,
		SalesOrderLineID: in.SalesOrderLineID,
		Qty:              in.Qty,
		ContainerLineID:  in.ContainerLineID,
	})
}

func (a *deliveryFGTrackerAdapter) ReleaseOnDelete(ctx context.Context, containerLineID uuid.UUID) error {
	return a.svc.ReleaseOnContainerDelete(ctx, containerLineID)
}

func (a *deliveryFGTrackerAdapter) MarkLoadedOnSeal(ctx context.Context, containerID uuid.UUID) error {
	return a.svc.MarkLoadedOnSeal(ctx, containerID)
}

// deliveryCustomerSKUAdapter implements delivery.CustomerSKUResolver by
// delegating to sales.GetCustomerSKUMapping. Trims the full mapping row down
// to just the sku_id the parser needs so the cross-module surface stays
// minimal — extending the contract requires touching this one adapter only.
type deliveryCustomerSKUAdapter struct{ svc sales.Service }

func (a *deliveryCustomerSKUAdapter) ResolveCustomerSKU(ctx context.Context, customerID uuid.UUID, code string) (uuid.UUID, error) {
	m, err := a.svc.GetCustomerSKUMapping(ctx, customerID, code)
	if err != nil {
		return uuid.Nil, err
	}
	return m.SKUID, nil
}

// deliveryLoadingPlanAuditAdapter implements delivery.LoadingPlanAuditLogger
// by writing directly to inventory_audit_log via pgxpool. Inlined here so the
// delivery module does not depend on inventory — the audit table is a shared
// cross-module ledger keyed by entity_type. Errors are swallowed by the
// service (best-effort): a transient audit-write failure must never roll
// back the loading-plan upload or approve.
//
// LP_* rows carry entity_type="LOADING_PLAN", entity_id = plan id; the
// metadata JSON column holds container_id + version + excel_hash so
// accountants can reconstruct the change without joining other tables.
type deliveryLoadingPlanAuditAdapter struct{ pool *pgxpool.Pool }

func (a *deliveryLoadingPlanAuditAdapter) LogLoadingPlan(ctx context.Context, in delivery.AuditLoadingPlanInput) error {
	meta, err := json.Marshal(map[string]any{
		"container_id": in.ContainerID,
		"version":      in.Version,
		"excel_hash":   in.ExcelHash,
	})
	if err != nil {
		return err
	}
	_, err = a.pool.Exec(ctx,
		`INSERT INTO inventory_audit_log
		    (id, entity_type, entity_id, action, actor_id, reason, metadata, created_at)
		 VALUES (gen_random_uuid(), 'LOADING_PLAN', $1, $2, $3, $4, $5, NOW())`,
		in.PlanID, string(in.Action), in.ActorID, in.Notes, meta,
	)
	return err
}
