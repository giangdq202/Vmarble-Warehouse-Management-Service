package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/module/authn"
	"github.com/vmarble/warehouse-management-service/internal/module/barcode"
	"github.com/vmarble/warehouse-management-service/internal/module/catalog"
	"github.com/vmarble/warehouse-management-service/internal/module/costing"
	"github.com/vmarble/warehouse-management-service/internal/module/dashboard"
	"github.com/vmarble/warehouse-management-service/internal/module/inventory"
	"github.com/vmarble/warehouse-management-service/internal/module/order"
	"github.com/vmarble/warehouse-management-service/internal/module/planning"
	"github.com/vmarble/warehouse-management-service/internal/module/production"
	"github.com/vmarble/warehouse-management-service/internal/module/purchasing"
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

	// ── Module services ─────────────────────────────────────
	authnSvc := authn.NewService(authnStore, cfg.AuthSecret)
	catalogSvc := catalog.NewService(catalogStore)
	orderSvc := order.NewService(orderStore)
	planningSvc := planning.NewService(planningStore)
	// woAdvanceAdapter is wired after productionSvc is constructed to avoid a
	// construction-time cycle (inventory → production → inventory).
	// costingChecker is similarly wired after costingSvc is constructed to avoid
	// production → costing → production cycle.
	woAdvance := &woAdvanceAdapter{}
	costingChecker := &costingCheckerAdapter{}
	barcodeGen := &cutBarcodeAdapter{planSvc: planningSvc}
	inventorySvc := inventory.NewServiceWithOverflowThreshold(
		inventoryStore,
		woAdvance,
		cfg.RemnantOverflowThresholdPct,
		barcodeGen,
	)

	productionSvc := production.NewService(
		productionStore,
		&planAdapter{svc: planningSvc},
		&skuAdapter{svc: catalogSvc},
		&userAdapter{svc: authnSvc},
		&sheetAssignAdapter{svc: inventorySvc},
		costingChecker,
		eventPublisher,
	)
	barcodeSvc := barcode.NewService(
		barcodeStore,
		&barcodeWOGatewayAdapter{svc: productionSvc},
		&barcodeUserLookupAdapter{svc: authnSvc},
	)

	// Wire production into the advance adapter now that it exists.
	woAdvance.svc = productionSvc
	barcodeGen.skuSvc = catalogSvc
	barcodeGen.woSvc = productionSvc
	barcodeGen.barcodeSvc = barcodeSvc

	costingSvc := costing.NewService(
		costingStore,
		&woAdapter{svc: productionSvc},
		&cuttingAdapter{pool: pool},
		&consumptionAdapter{pool: pool},
	)
	// Wire costing into the checker adapter now that it exists.
	costingChecker.svc = costingSvc
	dashboardSvc := dashboard.NewService(dashboardStore)
	purchasingSvc := purchasing.NewService(
		purchasingStore,
		&purchasingMaterialAdapter{svc: catalogSvc},
		&purchasingStockAdapter{svc: inventorySvc},
	)

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
	return production.SKUInfo{ID: s.ID, RequiresMetal: s.RequiresMetal}, nil
}

type userAdapter struct {
	svc authn.Service
}

func (a *userAdapter) GetUser(ctx context.Context, userID uuid.UUID) (production.UserInfo, error) {
	u, err := a.svc.GetUser(ctx, userID)
	if err != nil {
		return production.UserInfo{}, err
	}
	return production.UserInfo{ID: u.ID, Role: u.Role}, nil
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

func (a *sheetAssignAdapter) PreAssignSheet(ctx context.Context, sheetID uuid.UUID, workOrderID uuid.UUID) error {
	return a.svc.PreAssignSheet(ctx, sheetID, workOrderID)
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

	wip, err := a.barcodeSvc.GenerateBarcode(ctx, barcode.GenerateBarcodeInput{
		WorkOrderID:      in.WorkOrderID,
		SKUID:            wo.SKUID,
		POID:             plan.POID,
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
			POID:             plan.POID,
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
		MaterialID:  in.MaterialID,
		Dimensions:  domain.Dimension{LengthMM: in.LengthMM, WidthMM: in.WidthMM},
		CostPerSheet: in.UnitCost,
		Quantity:    in.Quantity,
		SupplierRef: in.SupplierRef,
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
