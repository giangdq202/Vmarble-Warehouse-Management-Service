package packing

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s            store
	barcodeIss   BarcodeIssuer
	barcodeRes   BarcodeResolver
	wog          WorkOrderGateway
	cs           ContainerSuggester
	clr          ContainerLineRemover
	notifier     DefectNotifier
	solChecker   SOLineChecker
	skuCompRes   SKUComponentResolver
	now          func() time.Time
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

// SetSOLineChecker wires the cross-module SOL validator after construction.
// Called from main.go once salesSvc is available.
func (svc *service) SetSOLineChecker(c SOLineChecker) {
	svc.solChecker = c
}

// SetSKUComponentResolver wires the catalog component resolver after construction.
// When nil, CreateFromCompletedWO creates one FG row per unit (single-box SKUs).
func (svc *service) SetSKUComponentResolver(r SKUComponentResolver) {
	svc.skuCompRes = r
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

	// BR-PK-MULTI01: resolve components; nil resolver or empty result → single-box.
	var components []SKUComponentInfo
	if svc.skuCompRes != nil {
		components, err = svc.skuCompRes.GetSKUComponents(ctx, in.SKUID)
		if err != nil {
			return nil, err
		}
	}

	now := svc.now()
	var rows []FGPool

	if len(components) == 0 {
		// Simple SKU: one FG row per physical unit.
		rows = make([]FGPool, 0, in.Quantity)
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
	} else {
		// BR-PK-MULTI01: multi-component SKU — one FG row per component per unit.
		// All components of the same physical unit share the same unit_index.
		rows = make([]FGPool, 0, in.Quantity*len(components))
		for unitIdx := 0; unitIdx < in.Quantity; unitIdx++ {
			for _, comp := range components {
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
				ct := comp.ComponentType
				ui := unitIdx
				rows = append(rows, FGPool{
					ID:               uuid.New(),
					WorkOrderID:      in.WorkOrderID,
					SKUID:            in.SKUID,
					BarcodeID:        bc.ID,
					SalesOrderLineID: in.SalesOrderLineID,
					Status:           FGStatusAvailable,
					ComponentType:    &ct,
					UnitIndex:        &ui,
					QCPassedAt:       now,
					QCPassedBy:       in.QCPassedBy,
					CreatedAt:        now,
				})
			}
		}
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
		if wo.Status != string(domain.WOCompleted) && wo.Status != string(domain.WOPartialComplete) && wo.Status != string(domain.WOCosted) {
			return ScanResult{}, domain.NewBizError(domain.ErrPreconditionFailed,
				"work order is "+wo.Status+", scan only valid after COMPLETED or PARTIAL_COMPLETE")
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
// BR-PK03. After commit, the suggestion engine checks for same-SKU
// replacements in the pool.
func (svc *service) ReportDefect(ctx context.Context, in ReportDefectInput) (DefectReportResult, error) {
	if in.BarcodeID == uuid.Nil || in.DetectedBy == uuid.Nil {
		return DefectReportResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"barcode_id and detected_by are required")
	}
	if !validDefectReason(in.Reason) {
		return DefectReportResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"invalid reason; expected BROKEN/WRONG_SIZE/MISSING_ACCESSORY/SCRATCHED/OTHER")
	}

	var defect FGDefect
	var skuCode string
	var hadContainerLine bool
	var fgSKUID uuid.UUID
	var fgID uuid.UUID
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
			hadContainerLine = true
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
		fgSKUID = fg.SKUID
		fgID = fg.ID
		return nil
	})
	if err != nil {
		return DefectReportResult{}, err
	}

	if svc.notifier != nil {
		// Best-effort: log via notifier impl, never fail the request.
		_ = svc.notifier.NotifyFGDefect(ctx, defect.FGPoolID, skuCode, defect.Reason)
	}

	// Shortfall suggestion engine: only when the FG was on a container
	// (i.e. it had a container_line_id → was contributing to a shipment).
	suggestions := svc.buildShortfallSuggestions(ctx, hadContainerLine, fgSKUID, fgID, skuCode)

	return DefectReportResult{Defect: defect, Suggestions: suggestions}, nil
}

// buildShortfallSuggestions returns actionable suggestions after a defect.
// If the FG was not on a container, there is no shortfall → empty.
// Otherwise, check the pool for same-SKU AVAILABLE replacements.
func (svc *service) buildShortfallSuggestions(ctx context.Context, hadContainerLine bool, skuID, excludeFGID uuid.UUID, skuCode string) []ShortfallSuggestion {
	if !hadContainerLine {
		return nil
	}

	const maxSuggestions = 5
	candidates, err := svc.s.selectAvailableFGsBySKU(ctx, skuID, excludeFGID, maxSuggestions)
	if err != nil {
		// Best-effort: if the query fails, return carry-over suggestion.
		return []ShortfallSuggestion{{
			Type:   SuggestionCarryOverWO,
			Detail: "Pool query failed; suggest carry-over WO for SKU " + skuCode,
			SKUID:  skuID,
		}}
	}

	if len(candidates) == 0 {
		return []ShortfallSuggestion{{
			Type:   SuggestionCarryOverWO,
			Detail: "No available FG in pool for SKU " + skuCode + "; suggest new carry-over WO",
			SKUID:  skuID,
		}}
	}

	suggestions := make([]ShortfallSuggestion, 0, len(candidates))
	for i := range candidates {
		id := candidates[i].ID
		suggestions = append(suggestions, ShortfallSuggestion{
			Type:   SuggestionReassign,
			Detail: "FG " + id.String()[:8] + " available (same SKU " + skuCode + ")",
			FGID:   &id,
			SKUID:  skuID,
		})
	}
	return suggestions
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
		if err := tx.updateDefectResolution(ctx, updateResolutionInput(in)); err != nil {
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

// ReassignFG moves an FG's soft-allocation to a different SO line.
// Allowed only when status is AVAILABLE or RESERVED (not LOADED/DEFECT/DISPOSED).
// The target SOL must reference the same SKU.
func (svc *service) ReassignFG(ctx context.Context, in ReassignFGInput) (ReassignFGResult, error) {
	if in.FGID == uuid.Nil {
		return ReassignFGResult{}, domain.NewBizError(domain.ErrInvalidInput, "fg_id is required")
	}
	if in.NewSOLineID == uuid.Nil {
		return ReassignFGResult{}, domain.NewBizError(domain.ErrInvalidInput, "new_sales_order_line_id is required")
	}
	if in.Reason == "" {
		return ReassignFGResult{}, domain.NewBizError(domain.ErrInvalidInput, "reason is required")
	}
	if in.ActorID == uuid.Nil {
		return ReassignFGResult{}, domain.NewBizError(domain.ErrInvalidInput, "actor_id is required")
	}

	// Validate target SOL exists and get its SKU for mismatch check.
	var targetSKUID uuid.UUID
	if svc.solChecker != nil {
		sol, err := svc.solChecker.GetSOLine(ctx, in.NewSOLineID)
		if err != nil {
			return ReassignFGResult{}, err
		}
		targetSKUID = sol.SKUID
	}

	var result ReassignFGResult
	err := svc.s.withTx(ctx, func(tx txStore) error {
		fg, err := tx.lockFGForUpdate(ctx, in.FGID)
		if err != nil {
			return err
		}

		switch fg.Status {
		case FGStatusAvailable, FGStatusReserved:
			// allowed
		case FGStatusLoaded:
			return domain.NewBizError(domain.ErrPreconditionFailed,
				"cannot reassign FG already loaded in a sealed container")
		default:
			return domain.NewBizError(domain.ErrInvalidTransition,
				"cannot reassign FG with status "+fg.Status)
		}

		// SKU mismatch check — only enforced when solChecker is wired.
		if svc.solChecker != nil && targetSKUID != fg.SKUID {
			return domain.NewBizError(domain.ErrInvalidInput,
				"cannot reassign to a different SKU")
		}

		// No-op reassign to same SOL is allowed (idempotent).
		if fg.SalesOrderLineID != nil && *fg.SalesOrderLineID == in.NewSOLineID {
			result = ReassignFGResult{FG: fg}
			return nil
		}

		now := svc.now()
		log := FGReassignmentLog{
			ID:           uuid.New(),
			FGID:         fg.ID,
			FromSOLID:    fg.SalesOrderLineID,
			ToSOLID:      in.NewSOLineID,
			ActorID:      in.ActorID,
			Reason:       in.Reason,
			ReassignedAt: now,
		}
		if err := tx.updateFGSOLine(ctx, fg.ID, &in.NewSOLineID); err != nil {
			return err
		}
		if err := tx.insertReassignLog(ctx, log); err != nil {
			return err
		}
		fg.SalesOrderLineID = &in.NewSOLineID
		result = ReassignFGResult{FG: fg, Audit: log}
		return nil
	})
	return result, err
}

// CheckComponentsForSeal validates BR-PK-MULTI03: every physical unit (grouped
// by unit_index) in the container's RESERVED FG rows must have all expected
// component_types present. Simple SKUs (unit_index IS NULL) are always valid.
func (svc *service) CheckComponentsForSeal(ctx context.Context, containerID uuid.UUID) error {
	if svc.skuCompRes == nil {
		return nil // component resolver not wired — skip (simple SKUs only)
	}

	fgs, err := svc.s.selectReservedFGsByContainer(ctx, containerID)
	if err != nil {
		return err
	}

	// Group multi-component FGs by (skuID, unitIndex).
	type unitKey struct {
		skuID     uuid.UUID
		unitIndex int
	}
	present := make(map[unitKey]map[string]struct{})
	for _, fg := range fgs {
		if fg.UnitIndex == nil || fg.ComponentType == nil {
			continue
		}
		k := unitKey{fg.SKUID, *fg.UnitIndex}
		if present[k] == nil {
			present[k] = make(map[string]struct{})
		}
		present[k][*fg.ComponentType] = struct{}{}
	}

	if len(present) == 0 {
		return nil
	}

	// Fetch expected components per SKU (cached across units of same SKU).
	skuComps := make(map[uuid.UUID][]SKUComponentInfo)
	var missingUnits []map[string]any

	for k, gotTypes := range present {
		comps, ok := skuComps[k.skuID]
		if !ok {
			comps, err = svc.skuCompRes.GetSKUComponents(ctx, k.skuID)
			if err != nil {
				return err
			}
			skuComps[k.skuID] = comps
		}
		var missing []string
		for _, c := range comps {
			if _, found := gotTypes[c.ComponentType]; !found {
				missing = append(missing, c.ComponentType)
			}
		}
		if len(missing) > 0 {
			missingUnits = append(missingUnits, map[string]any{
				"sku_id":     k.skuID,
				"unit_index": k.unitIndex,
				"missing":    missing,
			})
		}
	}

	if len(missingUnits) > 0 {
		return domain.NewBizError(domain.ErrPreconditionFailed,
			"cannot seal: some units are missing component packages (BR-PK-MULTI03)").
			WithDetails(map[string]any{"incomplete_units": missingUnits})
	}
	return nil
}
