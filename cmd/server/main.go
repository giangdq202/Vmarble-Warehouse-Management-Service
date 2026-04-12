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
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/module/barcode"
	"github.com/vmarble/warehouse-management-service/internal/module/authn"
	"github.com/vmarble/warehouse-management-service/internal/module/catalog"
	"github.com/vmarble/warehouse-management-service/internal/module/costing"
	"github.com/vmarble/warehouse-management-service/internal/module/inventory"
	"github.com/vmarble/warehouse-management-service/internal/module/order"
	"github.com/vmarble/warehouse-management-service/internal/module/planning"
	"github.com/vmarble/warehouse-management-service/internal/module/production"
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
	authnStore   := authn.NewPGStore(pool)
	catalogStore := catalog.NewPGStore(pool)
	orderStore := order.NewPGStore(pool)
	planningStore := planning.NewPGStore(pool)
	inventoryStore := inventory.NewPGStore(pool)
	productionStore := production.NewPGStore(pool)
	costingStore := costing.NewPGStore(pool)
	barcodeStore := barcode.NewPGStore(pool)

	// ── Module services ─────────────────────────────────────
	authnSvc  := authn.NewService(authnStore, cfg.AuthSecret)
	catalogSvc := catalog.NewService(catalogStore)
	orderSvc := order.NewService(orderStore)
	planningSvc := planning.NewService(planningStore)
	inventorySvc := inventory.NewService(inventoryStore)

	productionSvc := production.NewService(
		productionStore,
		&planAdapter{svc: planningSvc},
		&skuAdapter{svc: catalogSvc},
		&userAdapter{svc: authnSvc},
		eventPublisher,
	)

	costingSvc := costing.NewService(
		costingStore,
		&woAdapter{svc: productionSvc},
		&cuttingAdapter{pool: pool},
		&consumptionAdapter{pool: pool},
	)

	barcodeSvc := barcode.NewService(barcodeStore)

	// ── Gin router ──────────────────────────────────────────
	r := httpkit.NewRouter()

	// Swagger UI: /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ── Public routes (no auth required) ───────────────────
	public := r.Group("/api/auth")
	authn.NewHandler(authnSvc).Register(public)

	// ── Protected routes ────────────────────────────────────
	api := r.Group("/api/v1")
	api.Use(auth.Middleware(cfg.AuthSecret))

	catalog.NewHandler(catalogSvc).Register(api)
	order.NewHandler(orderSvc).Register(api)
	planning.NewHandler(planningSvc).Register(api)
	inventory.NewHandler(inventorySvc).Register(api)
	production.NewHandler(productionSvc, &sheetAdapter{svc: inventorySvc}).Register(api)
	costing.NewHandler(costingSvc).Register(api)
	barcode.NewHandler(barcodeSvc).Register(api)
	events.NewHandler(eventBroker).Register(api)

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
	return production.PlanInfo{ID: p.ID, Status: p.Status}, nil
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

type sheetAdapter struct {
	svc inventory.Service
}

func (a *sheetAdapter) PreassignSheet(ctx context.Context, sheetID uuid.UUID, workOrderID uuid.UUID) error {
	return a.svc.PreassignSheet(ctx, sheetID, workOrderID)
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
