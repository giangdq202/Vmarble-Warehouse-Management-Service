package packing

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s          store
	barcodeIss BarcodeIssuer
	barcodeRes BarcodeResolver
	wog        WorkOrderGateway
	cs         ContainerSuggester
	clr        ContainerLineRemover
	notifier   DefectNotifier
	now        func() time.Time
}

// NewService wires the packing module. Any cross-module dep may be nil in
// tests — the service guards each call site that uses one. The barcode
// issuer and resolver share the same module (barcode.Service); the rest are
// distinct adapters.
func NewService(
	s store,
	barcodeIss BarcodeIssuer,
	barcodeRes BarcodeResolver,
	wog WorkOrderGateway,
	cs ContainerSuggester,
	clr ContainerLineRemover,
	notifier DefectNotifier,
) Service {
	return &service{
		s:          s,
		barcodeIss: barcodeIss,
		barcodeRes: barcodeRes,
		wog:        wog,
		cs:         cs,
		clr:        clr,
		notifier:   notifier,
		now:        time.Now,
	}
}

// ── FG creation hook ────────────────────────────────────────────────────────

// CreateFromCompletedWO is idempotent: when the WO already has fg_pool rows,
// the existing rows are returned and no barcodes are generated. Production
// calls this from AdvanceStatus(COMPLETED) as best-effort — the AdvanceStatus
// transaction does not roll back if this returns an error.
func (svc *service) CreateFromCompletedWO(ctx context.Context, in CreateFromCompletedWOInput) ([]FGPool, error) {
	if in.WorkOrderID == uuid.Nil || in.SKUID == uuid.Nil || in.QCPassedBy == uuid.Nil {
		return nil, domain.NewBizError(domain.ErrInvalidInput,
			"work_order_id, sku_id, and qc_passed_by are required")
	}
	if in.Quantity <= 0 {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "quantity must be > 0")
	}

	existing, err := svc.s.selectFGByWorkOrderID(ctx, in.WorkOrderID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return existing, nil
	}

	if svc.barcodeIss == nil {
		return nil, domain.NewBizError(domain.ErrPreconditionFailed,
			"barcode issuer not configured")
	}

	now := svc.now()
	rows := make([]FGPool, 0, in.Quantity)
	for i := 0; i < in.Quantity; i++ {
		bc, err := svc.barcodeIss.GenerateBarcode(ctx, BarcodeIssueInput{
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
			return nil, err
		}
		rows = append(rows, FGPool{
			ID:               uuid.New(),
			WorkOrderID:      in.WorkOrderID,
			SKUID:            in.SKUID,
			BarcodeID:        bc.ID,
			SalesOrderLineID: in.SalesOrderLineID,
			Status:           FGStatusAvailable,
			QCPassedAt:       now,
			QCPassedBy:       in.QCPassedBy,
			CreatedAt:        now,
		})
	}
	if err := svc.s.insertFGBatch(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// ── FG reads ────────────────────────────────────────────────────────────────

func (svc *service) GetFG(ctx context.Context, id uuid.UUID) (FGPool, error) {
	return svc.s.selectFGByID(ctx, id)
}

func (svc *service) ListFG(ctx context.Context, p httpkit.PageParams, f FGListFilter) (httpkit.PagedResult[FGPool], error) {
	items, total, err := svc.s.selectFGPaged(ctx, p, f)
	if err != nil {
		return httpkit.PagedResult[FGPool]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

// ── Scan ────────────────────────────────────────────────────────────────────

// ScanBarcode resolves a barcode and returns the FG plus suggested loadable
// containers. BR-PK01: the underlying WO must be COMPLETED — otherwise the
// kiosk should never see the FG (production has not yet QC'd it).
func (svc *service) ScanBarcode(ctx context.Context, barcodeID, _ uuid.UUID) (ScanResult, error) {
	if barcodeID == uuid.Nil {
		return ScanResult{}, domain.NewBizError(domain.ErrInvalidInput, "barcode_id is required")
	}
	if svc.barcodeRes != nil {
		// Resolve to validate the barcode exists; the FG row may not be in
		// fg_pool yet if a race let the scan land before the WO COMPLETED
		// hook. Treat the missing FG as ErrNotFound so the kiosk shows the
		// correct "barcode unknown" message.
		if _, err := svc.barcodeRes.LookupBarcode(ctx, barcodeID); err != nil {
			return ScanResult{}, err
		}
	}

	fg, err := svc.s.selectFGByBarcodeID(ctx, barcodeID)
	if err != nil {
		return ScanResult{}, err
	}

	if svc.wog != nil {
		wo, err := svc.wog.GetWorkOrderStatus(ctx, fg.WorkOrderID)
		if err != nil {
			return ScanResult{}, err
		}
		if wo.Status != string(domain.WOCompleted) && wo.Status != string(domain.WOCosted) {
			return ScanResult{}, domain.NewBizError(domain.ErrPreconditionFailed,
				"work order is "+wo.Status+", scan only valid after COMPLETED")
		}
		var suggestions []ContainerSuggestion
		if svc.cs != nil && fg.SalesOrderLineID != nil && fg.Status == FGStatusAvailable {
			suggestions, err = svc.cs.SuggestForSOLine(ctx, *fg.SalesOrderLineID)
			if err != nil {
				return ScanResult{}, err
			}
		}
		return ScanResult{FG: fg, WOStatus: wo.Status, SuggestedContainers: suggestions}, nil
	}
	return ScanResult{FG: fg}, nil
}

// ── Defect ──────────────────────────────────────────────────────────────────

// ReportDefect flips the FG to DEFECT and inserts the defect row atomically.
// If the FG was RESERVED on a container line, that line is removed first via
// the ContainerLineRemover dep so the container's qty rebalances. BR-PK02 /
// BR-PK03.
func (svc *service) ReportDefect(ctx context.Context, in ReportDefectInput) (FGDefect, error) {
	if in.BarcodeID == uuid.Nil || in.DetectedBy == uuid.Nil {
		return FGDefect{}, domain.NewBizError(domain.ErrInvalidInput,
			"barcode_id and detected_by are required")
	}
	if !validDefectReason(in.Reason) {
		return FGDefect{}, domain.NewBizError(domain.ErrInvalidInput,
			"invalid reason; expected BROKEN/WRONG_SIZE/MISSING_ACCESSORY/SCRATCHED/OTHER")
	}

	var defect FGDefect
	var skuCode string
	err := svc.s.withTx(ctx, func(tx txStore) error {
		fg, err := tx.lockFGByBarcodeForUpdate(ctx, in.BarcodeID)
		if err != nil {
			return err
		}
		switch fg.Status {
		case FGStatusAvailable, FGStatusReserved:
			// allowed
		default:
			return domain.NewBizError(domain.ErrInvalidTransition,
				"FG is "+fg.Status+", defect only valid for AVAILABLE or RESERVED")
		}

		// BR-PK03: a RESERVED FG must be released from its container line
		// before flipping to DEFECT, so the line stops contributing to the
		// container's qty/cbm/weight totals.
		if fg.Status == FGStatusReserved && fg.ContainerLineID != nil {
			if svc.clr == nil {
				return domain.NewBizError(domain.ErrPreconditionFailed,
					"container line remover not configured; cannot release defective RESERVED FG")
			}
			if err := svc.clr.DeleteLineForDefect(ctx, *fg.ContainerLineID, in.DetectedBy); err != nil {
				return err
			}
		}

		if err := tx.flipFGStatus(ctx, flipStatusInput{
			FGID:            fg.ID,
			ToStatus:        FGStatusDefect,
			ContainerLineID: nil,
		}); err != nil {
			return err
		}

		now := svc.now()
		defect = FGDefect{
			ID:         uuid.New(),
			FGPoolID:   fg.ID,
			Reason:     in.Reason,
			Detail:     in.Detail,
			PhotoURLs:  in.PhotoURLs,
			DetectedBy: in.DetectedBy,
			DetectedAt: now,
		}
		if err := tx.insertDefect(ctx, defect); err != nil {
			return err
		}
		skuCode = fg.SKUCode
		return nil
	})
	if err != nil {
		return FGDefect{}, err
	}

	if svc.notifier != nil {
		// Best-effort: log via notifier impl, never fail the request.
		_ = svc.notifier.NotifyFGDefect(ctx, defect.FGPoolID, skuCode, defect.Reason)
	}
	return defect, nil
}

// ResolveDefect records the resolution audit columns and flips the FG status:
// DISCARD/RETURN_NCC -> DISPOSED, REWORK -> AVAILABLE so the FG can re-enter
// the pool after rework. v3 will create a supplemental WO for REWORK; v1
// just moves state.
func (svc *service) ResolveDefect(ctx context.Context, in ResolveDefectInput) (FGDefect, error) {
	if in.DefectID == uuid.Nil || in.ResolvedBy == uuid.Nil {
		return FGDefect{}, domain.NewBizError(domain.ErrInvalidInput,
			"defect_id and resolved_by are required")
	}
	if !validResolution(in.Resolution) {
		return FGDefect{}, domain.NewBizError(domain.ErrInvalidInput,
			"invalid resolution; expected DISCARD/REWORK/RETURN_NCC")
	}

	var resolved FGDefect
	err := svc.s.withTx(ctx, func(tx txStore) error {
		// Pre-load defect + FG; the audit update has its own
		// "AND resolution IS NULL" guard so a concurrent resolve loses
		// cleanly with ErrInvalidTransition.
		d, err := svc.s.selectDefectByID(ctx, in.DefectID)
		if err != nil {
			return err
		}
		fg, err := tx.lockFGForUpdate(ctx, d.FGPoolID)
		if err != nil {
			return err
		}
		if fg.Status != FGStatusDefect {
			return domain.NewBizError(domain.ErrInvalidTransition,
				"FG is "+fg.Status+", resolve only valid on DEFECT")
		}

		nextStatus := FGStatusDisposed
		if in.Resolution == DefectResolutionRework {
			nextStatus = FGStatusAvailable
		}
		if err := tx.flipFGStatus(ctx, flipStatusInput{
			FGID:            fg.ID,
			ToStatus:        nextStatus,
			ContainerLineID: nil,
		}); err != nil {
			return err
		}
		if err := tx.updateDefectResolution(ctx, updateResolutionInput{
			DefectID:   in.DefectID,
			Resolution: in.Resolution,
			Note:       in.Note,
			ResolvedBy: in.ResolvedBy,
		}); err != nil {
			return err
		}
		resolved = d
		resolved.Resolution = in.Resolution
		resolved.Note = in.Note
		now := svc.now()
		resolved.ResolvedAt = &now
		resolved.ResolvedBy = &in.ResolvedBy
		return nil
	})
	if err != nil {
		return FGDefect{}, err
	}

	if svc.notifier != nil {
		_ = svc.notifier.NotifyFGDefectResolved(ctx, resolved.FGPoolID, resolved.Resolution)
	}
	return resolved, nil
}

// ── Delivery hooks ──────────────────────────────────────────────────────────

// ReserveOnContainerAdd is called by delivery.AddLine inside the AddLine tx.
// It locks `qty` AVAILABLE FG rows matching the SO line and flips them to
// RESERVED. Returns the count actually reserved — soft allocation, the
// caller decides whether a shortfall is fatal (#291 v1 just logs the gap).
func (svc *service) ReserveOnContainerAdd(ctx context.Context, in ReserveInput) (int, error) {
	if in.SKUID == uuid.Nil || in.SalesOrderLineID == uuid.Nil || in.ContainerLineID == uuid.Nil {
		return 0, domain.NewBizError(domain.ErrInvalidInput,
			"sku_id, sales_order_line_id, and container_line_id are required")
	}
	if in.Qty <= 0 {
		return 0, nil
	}
	var reserved int
	err := svc.s.withTx(ctx, func(tx txStore) error {
		rows, err := tx.lockAvailableFGsForReserve(ctx, in.SKUID, in.SalesOrderLineID, in.Qty)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		clID := in.ContainerLineID
		if err := tx.bulkFlipFGStatus(ctx, ids, FGStatusReserved, &clID); err != nil {
			return err
		}
		reserved = len(rows)
		return nil
	})
	return reserved, err
}

// ReleaseOnContainerDelete flips every fg_pool row pointing at the deleted
// container line back to AVAILABLE so it can re-enter the pool. Idempotent —
// no-op when no FG rows reference the line (e.g. legacy lines without a
// packing pool entry).
func (svc *service) ReleaseOnContainerDelete(ctx context.Context, containerLineID uuid.UUID) error {
	if containerLineID == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "container_line_id is required")
	}
	return svc.s.withTx(ctx, func(tx txStore) error {
		rows, err := tx.lockReservedFGsByContainerLine(ctx, containerLineID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		return tx.bulkFlipFGStatus(ctx, ids, FGStatusAvailable, nil)
	})
}

// MarkLoadedOnSeal flips every RESERVED FG on the sealed container to
// LOADED. Idempotent: rows already LOADED are not touched (the lock query
// only picks RESERVED). Called from delivery.Seal after the container
// status flip commits.
func (svc *service) MarkLoadedOnSeal(ctx context.Context, containerID uuid.UUID) error {
	if containerID == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	return svc.s.withTx(ctx, func(tx txStore) error {
		rows, err := tx.lockReservedFGsByContainer(ctx, containerID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		// Each row keeps its existing container_line_id (we passed it
		// through bulkFlipFGStatus by leaving the pointer non-nil). But
		// bulkFlipFGStatus overwrites with the same value — handle by
		// setting status row-by-row so container_line_id is preserved.
		for _, r := range rows {
			cl := r.ContainerLineID
			if err := tx.flipFGStatus(ctx, flipStatusInput{
				FGID:            r.ID,
				ToStatus:        FGStatusLoaded,
				ContainerLineID: cl,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ── helpers ─────────────────────────────────────────────────────────────────

func validDefectReason(r string) bool {
	switch r {
	case DefectReasonBroken, DefectReasonWrongSize,
		DefectReasonMissingAccessory, DefectReasonScratched, DefectReasonOther:
		return true
	}
	return false
}

func validResolution(r string) bool {
	switch r {
	case DefectResolutionDiscard, DefectResolutionRework, DefectResolutionReturnNCC:
		return true
	}
	return false
}
