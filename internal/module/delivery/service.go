package delivery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s              store
	skuChecker     SKUChecker
	soLineChecker  SOLineChecker
	shipRecorder   ShipmentRecorder
	fgTracker      FGTracker
	skuResolver    CustomerSKUResolver
	lpAuditor      LoadingPlanAuditLogger
	planReloader   PlanReloadNotifier
	pendingExc     PendingExceptionsChecker
	shortShipped   ShortShippedAutoCreator
	cbmOverheadPct float64
	now            func() time.Time // overridable in tests
}

// NewService wires the delivery module against a store and the cross-module
// dependencies it consumes. Any of the cross-module deps may be nil in tests
// — the service guards each branch that uses them. cbmOverheadPct widens the
// BR-D02 / BR-D03 cap (5 = 5%); negative or zero values disable the overhead.
func NewService(s store, sku SKUChecker, soLine SOLineChecker, ship ShipmentRecorder, cbmOverheadPct float64) Service {
	return &service{
		s:              s,
		skuChecker:     sku,
		soLineChecker:  soLine,
		shipRecorder:   ship,
		cbmOverheadPct: cbmOverheadPct,
		now:            time.Now,
	}
}

// SetFGTracker wires the packing FG hooks after construction. Done as a
// setter (rather than a constructor parameter) because packing depends on
// delivery via ContainerSuggester / ContainerLineRemover, so the two
// services would otherwise form a construction cycle. main.go calls this
// once both services exist; tests leave it nil.
func (svc *service) SetFGTracker(t FGTracker) {
	svc.fgTracker = t
}

// SetCustomerSKUResolver wires the loading-plan parser dependency. Optional —
// when nil, UploadLoadingPlan returns ErrPreconditionFailed so the missing
// wiring is loud rather than a silent UNMAPPED_SKU storm.
func (svc *service) SetCustomerSKUResolver(r CustomerSKUResolver) {
	svc.skuResolver = r
}

// SetLoadingPlanAuditor wires the best-effort audit hook for upload/approve
// flows. nil leaves auditing off; production wires lpAuditAdapter.
func (svc *service) SetLoadingPlanAuditor(a LoadingPlanAuditLogger) {
	svc.lpAuditor = a
}

// SetPlanReloadNotifier wires the best-effort SSE hook fired after a v2
// supersede wipes container_lines (BR-D13). nil leaves notifications off.
func (svc *service) SetPlanReloadNotifier(n PlanReloadNotifier) {
	svc.planReloader = n
}

// SetPendingExceptionsChecker wires the BR-D14 SEAL pre-check (#303). nil
// disables the guard.
func (svc *service) SetPendingExceptionsChecker(c PendingExceptionsChecker) {
	svc.pendingExc = c
}

// SetShortShippedAutoCreator wires the BR-D15 hook (#303). nil disables
// SHORT_SHIPPED auto-creation at seal time.
func (svc *service) SetShortShippedAutoCreator(c ShortShippedAutoCreator) {
	svc.shortShipped = c
}

// ── Container CRUD ──────────────────────────────────────────────────────────

func (svc *service) CreateContainer(ctx context.Context, in CreateContainerInput) (Container, error) {
	defaultCBM, defaultPayload, ok := DefaultCapacityForType(in.ContainerType)
	if !ok {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput,
			"invalid container_type, expected one of 20GP / 40GP / 40HC")
	}
	if in.MaxCBM <= 0 {
		in.MaxCBM = defaultCBM
	}
	if in.MaxPayloadKG <= 0 {
		in.MaxPayloadKG = defaultPayload
	}
	if in.CreatedBy == uuid.Nil {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput, "created_by is required")
	}

	now := svc.now()
	code, err := svc.s.nextContainerCode(ctx, now)
	if err != nil {
		return Container{}, err
	}

	c := Container{
		ID:            uuid.New(),
		Code:          code,
		ContainerType: in.ContainerType,
		MaxCBM:        in.MaxCBM,
		MaxPayloadKG:  in.MaxPayloadKG,
		Status:        ContainerStatusOpen,
		Note:          in.Note,
		CreatedBy:     in.CreatedBy,
		CreatedAt:     now,
	}
	if err := svc.s.insertContainer(ctx, c); err != nil {
		return Container{}, err
	}
	return c, nil
}

func (svc *service) GetContainer(ctx context.Context, id uuid.UUID) (Container, error) {
	c, err := svc.s.selectContainerByID(ctx, id)
	if err != nil {
		return Container{}, err
	}
	lines, err := svc.s.selectContainerLines(ctx, id)
	if err != nil {
		return Container{}, err
	}
	c.Lines = lines
	for _, l := range lines {
		c.UsedCBM += l.CBMTotal
		c.UsedWeight += l.WeightKGTotal
	}
	if c.MaxCBM > 0 {
		c.FillPctCBM = roundPct(c.UsedCBM / c.MaxCBM * 100)
	}
	if c.MaxPayloadKG > 0 {
		c.FillPctMass = roundPct(c.UsedWeight / c.MaxPayloadKG * 100)
	}
	return c, nil
}

func (svc *service) ListContainers(ctx context.Context, p httpkit.PageParams, f ContainerListFilter) (httpkit.PagedResult[Container], error) {
	items, total, err := svc.s.selectContainersPaged(ctx, p, f)
	if err != nil {
		return httpkit.PagedResult[Container]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

func (svc *service) ListStatusLog(ctx context.Context, containerID uuid.UUID) ([]ContainerStatusLogEntry, error) {
	if _, err := svc.s.selectContainerByID(ctx, containerID); err != nil {
		return nil, err
	}
	return svc.s.selectStatusLog(ctx, containerID)
}

// ── Line operations ─────────────────────────────────────────────────────────

// AddLine honours BR-D01 (status guard), BR-D02/D03 (capacity guard), plus
// cross-module checks (SKU exists, SO line exists, qty fits within
// qty_planned - qty_shipped on the underlying SO line).
// BR-D18/D20: hard capacity limit returns ErrInsufficientStock (422).
// BR-D19: near-capacity warning at 90% sets AddLineResult.NearCapacity.
// Admin override: AllowOverload=true bypasses the hard limit and writes an
// audit row atomically (actor must be admin role).
func (svc *service) AddLine(ctx context.Context, in AddLineInput) (AddLineResult, error) {
	if in.Qty <= 0 {
		return AddLineResult{}, domain.NewBizError(domain.ErrInvalidInput, "qty must be > 0")
	}
	if in.SKUID == uuid.Nil || in.SalesOrderLineID == uuid.Nil {
		return AddLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"sku_id and sales_order_line_id are required")
	}
	if in.CBMTotal < 0 || in.WeightKGTotal < 0 {
		return AddLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"cbm_total and weight_kg_total must be non-negative")
	}
	if in.AddedBy == uuid.Nil {
		return AddLineResult{}, domain.NewBizError(domain.ErrInvalidInput, "added_by is required")
	}
	if in.AllowOverload && !isAdminRole(in.ActorRole) {
		return AddLineResult{}, domain.NewBizError(domain.ErrPreconditionFailed,
			"allow_overload requires admin role")
	}

	if svc.skuChecker != nil {
		if _, err := svc.skuChecker.GetSKU(ctx, in.SKUID); err != nil {
			return AddLineResult{}, err
		}
	}

	if svc.soLineChecker != nil {
		soLine, err := svc.soLineChecker.GetSOLine(ctx, in.SalesOrderLineID)
		if err != nil {
			return AddLineResult{}, err
		}
		if soLine.SKUID != in.SKUID {
			return AddLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
				"sku_id does not match the sales order line's SKU")
		}
		switch soLine.SOStatus {
		case "CONFIRMED", "IN_PRODUCTION", "PARTIALLY_SHIPPED":
			// allowed
		default:
			return AddLineResult{}, domain.NewBizError(domain.ErrInvalidTransition,
				"sales order must be CONFIRMED or later before loading: got "+soLine.SOStatus)
		}
		if soLine.QtyShipped+in.Qty > soLine.QtyPlanned {
			return AddLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
				"qty exceeds remaining planned quantity for sales order line")
		}
	}

	var line ContainerLine
	var nearCapacity bool
	err := svc.s.withTx(ctx, func(tx txStore, _ pgx.Tx) error {
		c, err := tx.lockContainerForUpdate(ctx, in.ContainerID)
		if err != nil {
			return err
		}
		switch c.Status {
		case ContainerStatusOpen, ContainerStatusLoading:
			// continue
		default:
			return domain.NewBizError(domain.ErrInvalidTransition,
				"container is "+c.Status+", cannot add lines")
		}

		curCBM, curWeight, err := tx.sumLinesAggregates(ctx, in.ContainerID)
		if err != nil {
			return err
		}
		projCBM := curCBM + in.CBMTotal
		projWeight := curWeight + in.WeightKGTotal

		capErr := svc.checkCapacity(c, projCBM, projWeight)
		if capErr != nil && !in.AllowOverload {
			return capErr
		}

		nearCapacity = svc.isNearCapacity(c, projCBM, projWeight)

		now := svc.now()
		line = ContainerLine{
			ID:               uuid.New(),
			ContainerID:      in.ContainerID,
			SKUID:            in.SKUID,
			Qty:              in.Qty,
			SalesOrderLineID: in.SalesOrderLineID,
			CBMTotal:         in.CBMTotal,
			WeightKGTotal:    in.WeightKGTotal,
			AddedBy:          in.AddedBy,
			AddedAt:          now,
		}
		if err := tx.insertLine(ctx, line); err != nil {
			return err
		}

		// Write admin override audit atomically with the line insert.
		if capErr != nil && in.AllowOverload {
			overlog := ContainerOverloadLog{
				ID:           uuid.New(),
				ContainerID:  c.ID,
				LineID:       line.ID,
				ProjectedCBM: projCBM,
				MaxCBM:       c.MaxCBM,
				ProjectedKG:  projWeight,
				MaxKG:        c.MaxPayloadKG,
				ActorID:      in.AddedBy,
				Reason:       "admin override",
			}
			if err := tx.insertOverloadLog(ctx, overlog); err != nil {
				return err
			}
		}

		// First line on an OPEN container flips it to LOADING. Subsequent
		// adds on LOADING leave the status alone.
		if c.Status == ContainerStatusOpen {
			if _, err := tx.updateContainerStatus(ctx, updateStatusInput{
				ContainerID: c.ID,
				FromStatus:  ContainerStatusOpen,
				ToStatus:    ContainerStatusLoading,
				ActorID:     in.AddedBy,
				Note:        "first line added",
				Now:         now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return AddLineResult{}, err
	}

	// Best-effort FG reservation: flip qty matching AVAILABLE rows in the
	// packing pool to RESERVED. Runs OUTSIDE the AddLine tx because packing
	// owns its own pool — a shortfall is logged but not fatal (manual
	// reconciliation via the FG list).
	if svc.fgTracker != nil {
		req := FGReserveRequest{
			SKUID:            line.SKUID,
			SalesOrderLineID: line.SalesOrderLineID,
			Qty:              line.Qty,
			ContainerLineID:  line.ID,
		}
		reserved, rerr := svc.fgTracker.ReserveOnAdd(ctx, req)
		if rerr != nil {
			slog.Warn("delivery: FG reserve failed",
				"line_id", line.ID, "qty", line.Qty, "err", rerr)
		} else if reserved < line.Qty {
			slog.Warn("delivery: FG reserve shortfall",
				"line_id", line.ID, "wanted", line.Qty, "reserved", reserved)
		}
	}
	return AddLineResult{Line: line, NearCapacity: nearCapacity}, nil
}

// isNearCapacity returns true when projected CBM or weight exceeds 90% of
// the container max (BR-D19 near-capacity warning threshold).
func (svc *service) isNearCapacity(c Container, projCBM, projWeight float64) bool {
	const nearPct = 0.90
	if c.MaxCBM > 0 && projCBM > c.MaxCBM*nearPct {
		return true
	}
	if c.MaxPayloadKG > 0 && projWeight > c.MaxPayloadKG*nearPct {
		return true
	}
	return false
}

// isAdminRole returns true when the role string maps to the ADMIN persona.
func isAdminRole(role string) bool {
	return role == "admin"
}

func (svc *service) DeleteLine(ctx context.Context, containerID, lineID uuid.UUID, _ uuid.UUID) error {
	if err := svc.s.withTx(ctx, func(tx txStore, _ pgx.Tx) error {
		c, err := tx.lockContainerForUpdate(ctx, containerID)
		if err != nil {
			return err
		}
		switch c.Status {
		case ContainerStatusOpen, ContainerStatusLoading:
			// continue
		default:
			return domain.NewBizError(domain.ErrInvalidTransition,
				"container is "+c.Status+", cannot remove lines")
		}
		line, err := tx.lockLineForUpdate(ctx, lineID)
		if err != nil {
			return err
		}
		if line.ContainerID != containerID {
			return domain.NewBizError(domain.ErrInvalidInput, "line does not belong to this container")
		}
		return tx.deleteLine(ctx, lineID)
	}); err != nil {
		return err
	}

	// Best-effort: release any FG that was RESERVED to this line back to AVAILABLE.
	if svc.fgTracker != nil {
		if err := svc.fgTracker.ReleaseOnDelete(ctx, lineID); err != nil {
			slog.Warn("delivery: FG release failed", "line_id", lineID, "err", err)
		}
	}
	return nil
}

// TransferLine moves part or all of a line into a target container. The
// transaction locks both containers in a deterministic order (lower UUID
// first) to avoid the deadlock where two foremen transfer between the same
// pair of containers in opposite directions. The line itself is locked AFTER
// both containers so the line lock waits inside the container critical
// section, never the other way around.
func (svc *service) TransferLine(ctx context.Context, in TransferLineInput) (TransferLineResult, error) {
	if in.LineID == uuid.Nil || in.TargetContainerID == uuid.Nil {
		return TransferLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"line_id and target_container_id are required")
	}
	if in.ContainerID == in.TargetContainerID {
		return TransferLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"target_container_id must differ from source")
	}
	if in.Qty < 0 {
		return TransferLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"qty must be non-negative")
	}
	// BR-D07: reason is mandatory for every transfer.
	if reasonBlank(in.Reason) {
		return TransferLineResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"reason is required for a container line transfer")
	}

	first, second := orderUUIDs(in.ContainerID, in.TargetContainerID)
	var result TransferLineResult
	err := svc.s.withTx(ctx, func(tx txStore, _ pgx.Tx) error {
		// Lock containers in canonical order, then resolve them back to
		// src / target so the rest of the function reads naturally.
		a, err := tx.lockContainerForUpdate(ctx, first)
		if err != nil {
			return err
		}
		b, err := tx.lockContainerForUpdate(ctx, second)
		if err != nil {
			return err
		}
		var src, target Container
		if a.ID == in.ContainerID {
			src, target = a, b
		} else {
			src, target = b, a
		}

		if !canHoldLines(src.Status) || !canHoldLines(target.Status) {
			return domain.NewBizError(domain.ErrInvalidTransition,
				"source and target containers must both be OPEN or LOADING")
		}

		// BR-D17: cross-plan transfer requires planner tier.
		// A transfer is cross-plan when the source container has an APPROVED
		// loading plan (i.e. a version-locked plan that workers scan against).
		isCrossPlan, err := tx.hasApprovedLoadingPlan(ctx, src.ID)
		if err != nil {
			return err
		}
		if isCrossPlan && !isPlannerOrAbove(in.ActorRole) {
			return domain.NewBizError(domain.ErrPreconditionFailed,
				"transferring lines out of a container with an approved loading plan requires the planner role or above")
		}

		line, err := tx.lockLineForUpdate(ctx, in.LineID)
		if err != nil {
			return err
		}
		if line.ContainerID != src.ID {
			return domain.NewBizError(domain.ErrInvalidInput,
				"line does not belong to the source container")
		}

		moveQty := in.Qty
		if moveQty == 0 || moveQty == line.Qty {
			// Full transfer: line-level move with the original snapshot.
			if err := svc.checkCapacityFromAggregate(ctx, tx, target, line.CBMTotal, line.WeightKGTotal); err != nil {
				return err
			}
			now := svc.now()
			newLine := ContainerLine{
				ID:               uuid.New(),
				ContainerID:      target.ID,
				SKUID:            line.SKUID,
				Qty:              line.Qty,
				SalesOrderLineID: line.SalesOrderLineID,
				CBMTotal:         line.CBMTotal,
				WeightKGTotal:    line.WeightKGTotal,
				AddedBy:          in.ActorID,
				AddedAt:          now,
			}
			if err := tx.insertLine(ctx, newLine); err != nil {
				return err
			}
			if err := tx.deleteLine(ctx, line.ID); err != nil {
				return err
			}
			if err := svc.flipToLoadingIfOpen(ctx, tx, target, in.ActorID); err != nil {
				return err
			}
			audit := ContainerTransferAudit{
				ID:                uuid.New(),
				SourceContainerID: src.ID,
				TargetContainerID: target.ID,
				LineID:            line.ID,
				SKUID:             line.SKUID,
				QtyTransferred:    line.Qty,
				Reason:            in.Reason,
				IsCrossPlan:       isCrossPlan,
				ActorID:           in.ActorID,
				ActorRole:         in.ActorRole,
				CreatedAt:         now,
			}
			if err := tx.insertTransferAudit(ctx, audit); err != nil {
				return err
			}
			result = TransferLineResult{TargetLine: newLine, Audit: audit}
			return nil
		}

		// Partial transfer.
		if moveQty > line.Qty {
			return domain.NewBizError(domain.ErrInvalidInput,
				"qty exceeds line.qty; cannot transfer more than the line carries")
		}
		if in.CBMTotal <= 0 || in.WeightKGTotal <= 0 {
			return domain.NewBizError(domain.ErrInvalidInput,
				"cbm_total and weight_kg_total are required for a partial transfer")
		}
		if in.CBMTotal > line.CBMTotal || in.WeightKGTotal > line.WeightKGTotal {
			return domain.NewBizError(domain.ErrInvalidInput,
				"partial cbm/weight cannot exceed the source line's snapshot")
		}

		if err := svc.checkCapacityFromAggregate(ctx, tx, target, in.CBMTotal, in.WeightKGTotal); err != nil {
			return err
		}

		remainingQty := line.Qty - moveQty
		remainingCBM := line.CBMTotal - in.CBMTotal
		remainingWeight := line.WeightKGTotal - in.WeightKGTotal
		if err := tx.updateLineQty(ctx, line.ID, remainingQty, remainingCBM, remainingWeight); err != nil {
			return err
		}
		now := svc.now()
		newLine := ContainerLine{
			ID:               uuid.New(),
			ContainerID:      target.ID,
			SKUID:            line.SKUID,
			Qty:              moveQty,
			SalesOrderLineID: line.SalesOrderLineID,
			CBMTotal:         in.CBMTotal,
			WeightKGTotal:    in.WeightKGTotal,
			AddedBy:          in.ActorID,
			AddedAt:          now,
		}
		if err := tx.insertLine(ctx, newLine); err != nil {
			return err
		}
		if err := svc.flipToLoadingIfOpen(ctx, tx, target, in.ActorID); err != nil {
			return err
		}
		audit := ContainerTransferAudit{
			ID:                uuid.New(),
			SourceContainerID: src.ID,
			TargetContainerID: target.ID,
			LineID:            line.ID,
			SKUID:             line.SKUID,
			QtyTransferred:    moveQty,
			Reason:            in.Reason,
			IsCrossPlan:       isCrossPlan,
			ActorID:           in.ActorID,
			ActorRole:         in.ActorRole,
			CreatedAt:         now,
		}
		if err := tx.insertTransferAudit(ctx, audit); err != nil {
			return err
		}
		// Source line stays in place with reduced qty.
		updated := line
		updated.Qty = remainingQty
		updated.CBMTotal = remainingCBM
		updated.WeightKGTotal = remainingWeight
		result = TransferLineResult{SourceLine: &updated, TargetLine: newLine, Audit: audit}
		return nil
	})
	if err != nil {
		return TransferLineResult{}, err
	}
	return result, nil
}

// isPlannerOrAbove returns true for the planner and admin persona tiers.
// Used by BR-D17 to gate cross-plan transfers.
func isPlannerOrAbove(roleStr string) bool {
	switch auth.Role(roleStr) {
	case auth.RolePlanner, auth.RoleAccountant, auth.RoleAdmin:
		return true
	}
	return false
}

// ── State transitions ───────────────────────────────────────────────────────

// Seal flips the container to SEALED and atomically bumps qty_shipped on the
// underlying sales_order_lines via the cross-module ShipmentRecorder, all in
// the same transaction. BR-D05.
//
// BR-D14: refuses with ErrPreconditionFailed when the container has any
// pending loading_exceptions (#303); the response body lists the blocking
// ids so the FE can deep-link to the inbox.
//
// BR-D15: after a successful seal, auto-creates SHORT_SHIPPED exceptions
// for every (sku) where actual loaded qty < planned qty in the active
// loading plan. The auto-create is best-effort — a failure is logged but
// does not roll back the seal because the bump already committed.
func (svc *service) Seal(ctx context.Context, in SealInput) (Container, error) {
	if svc.shipRecorder == nil {
		return Container{}, domain.NewBizError(domain.ErrPreconditionFailed,
			"shipment recorder is not configured")
	}
	// BR-D14: pre-check pending exceptions. Done outside the tx because the
	// loading_exception module owns its own pool and the read is cheap.
	if svc.pendingExc != nil {
		summary, err := svc.pendingExc.PendingForContainer(ctx, in.ContainerID)
		if err != nil {
			return Container{}, err
		}
		if summary.Count > 0 {
			return Container{}, domain.NewBizError(domain.ErrPreconditionFailed,
				"cannot seal: container has pending loading exceptions").
				WithDetails(map[string]any{
					"pending_exception_ids": summary.IDs,
					"pending_count":         summary.Count,
				})
		}
	}
	var sealed Container
	err := svc.s.withTx(ctx, func(tx txStore, raw pgx.Tx) error {
		c, err := tx.lockContainerForUpdate(ctx, in.ContainerID)
		if err != nil {
			return err
		}
		switch c.Status {
		case ContainerStatusOpen, ContainerStatusLoading:
			// continue
		default:
			return domain.NewBizError(domain.ErrInvalidTransition,
				"only OPEN/LOADING containers can be sealed; got "+c.Status)
		}

		// BR-D08: cannot seal after vessel cutoff_date.
		if c.CutoffDate != nil && svc.now().After(*c.CutoffDate) {
			return domain.NewBizError(domain.ErrPreconditionFailed,
				"cannot seal: vessel cutoff_date has passed").
				WithDetails(map[string]any{
					"cutoff_date": c.CutoffDate,
				})
		}

		items, err := tx.listLinesForSeal(ctx, in.ContainerID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return domain.NewBizError(domain.ErrInvalidInput,
				"cannot seal an empty container")
		}

		shipItems := make([]ShipmentItem, len(items))
		copy(shipItems, items)

		if err := svc.shipRecorder.RecordShipmentTx(ctx, raw, shipItems); err != nil {
			return err
		}

		now := svc.now()
		updated, err := tx.updateContainerStatus(ctx, updateStatusInput{
			ContainerID: c.ID,
			FromStatus:  c.Status,
			ToStatus:    ContainerStatusSealed,
			ActorID:     in.ActorID,
			Note:        in.Note,
			Now:         now,
		})
		if err != nil {
			return err
		}
		sealed = updated
		return nil
	})
	if err != nil {
		return Container{}, err
	}

	// Best-effort FG mark-loaded: flip every RESERVED FG on this container
	// to LOADED so dashboards stop counting it as in-flight inventory.
	if svc.fgTracker != nil {
		if err := svc.fgTracker.MarkLoadedOnSeal(ctx, in.ContainerID); err != nil {
			slog.Warn("delivery: FG mark-loaded failed", "container_id", in.ContainerID, "err", err)
		}
	}

	// BR-D15: auto-create SHORT_SHIPPED loading_exceptions when actual loaded
	// qty is below the active loading plan. Best-effort post-commit because
	// the seal already happened — a failure here would force the operator to
	// manually raise the variance, not roll back qty_shipped on the SO lines.
	if svc.shortShipped != nil {
		report, err := svc.s.selectShortagesForContainer(ctx, in.ContainerID)
		if err != nil {
			slog.Warn("delivery: shortage read failed", "container_id", in.ContainerID, "err", err)
		} else {
			for _, item := range report.Items {
				if item.MissingQty <= 0 {
					continue
				}
				if err := svc.shortShipped.AutoCreateShortShipped(ctx, ShortShippedAutoInput{
					ContainerID:   in.ContainerID,
					LoadingPlanID: report.LoadingPlanID,
					SKUID:         item.SKUID,
					MissingQty:    item.MissingQty,
					ActorID:       in.ActorID,
				}); err != nil {
					slog.Warn("delivery: SHORT_SHIPPED auto-create failed",
						"container_id", in.ContainerID, "sku_id", item.SKUID, "err", err)
				}
			}
		}
	}
	return sealed, nil
}

// Reopen rolls SEALED → LOADING for admin-only correction flows. BR-D06
// requires a non-empty reason which is recorded in the audit log.
func (svc *service) Reopen(ctx context.Context, in ReopenInput) (Container, error) {
	if reasonBlank(in.Reason) {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput,
			"reason is required when reopening a sealed container")
	}
	var out Container
	err := svc.s.withTx(ctx, func(tx txStore, _ pgx.Tx) error {
		c, err := tx.lockContainerForUpdate(ctx, in.ContainerID)
		if err != nil {
			return err
		}
		if c.Status != ContainerStatusSealed {
			return domain.NewBizError(domain.ErrInvalidTransition,
				"only SEALED containers can be reopened; got "+c.Status)
		}
		updated, err := tx.updateContainerStatus(ctx, updateStatusInput{
			ContainerID: c.ID,
			FromStatus:  ContainerStatusSealed,
			ToStatus:    ContainerStatusLoading,
			ActorID:     in.ActorID,
			Note:        in.Reason,
			Now:         svc.now(),
		})
		if err != nil {
			return err
		}
		out = updated
		return nil
	})
	return out, err
}

// Ship flips SEALED → SHIPPED. The qty_shipped bump already happened at seal
// time so SO status recompute is not re-driven here; ship is purely a
// transit signal for the warehouse.
func (svc *service) Ship(ctx context.Context, in ShipInput) (Container, error) {
	var out Container
	err := svc.s.withTx(ctx, func(tx txStore, _ pgx.Tx) error {
		c, err := tx.lockContainerForUpdate(ctx, in.ContainerID)
		if err != nil {
			return err
		}
		if c.Status != ContainerStatusSealed {
			return domain.NewBizError(domain.ErrInvalidTransition,
				"only SEALED containers can be shipped; got "+c.Status)
		}
		updated, err := tx.updateContainerStatus(ctx, updateStatusInput{
			ContainerID: c.ID,
			FromStatus:  ContainerStatusSealed,
			ToStatus:    ContainerStatusShipped,
			ActorID:     in.ActorID,
			Note:        in.Note,
			Now:         svc.now(),
		})
		if err != nil {
			return err
		}
		out = updated
		return nil
	})
	return out, err
}

func (svc *service) Cancel(ctx context.Context, in CancelInput) (Container, error) {
	var out Container
	err := svc.s.withTx(ctx, func(tx txStore, _ pgx.Tx) error {
		c, err := tx.lockContainerForUpdate(ctx, in.ContainerID)
		if err != nil {
			return err
		}
		switch c.Status {
		case ContainerStatusOpen, ContainerStatusLoading:
			// allowed
		default:
			return domain.NewBizError(domain.ErrInvalidTransition,
				"only OPEN/LOADING containers can be cancelled; got "+c.Status)
		}
		updated, err := tx.updateContainerStatus(ctx, updateStatusInput{
			ContainerID: c.ID,
			FromStatus:  c.Status,
			ToStatus:    ContainerStatusCancelled,
			ActorID:     in.ActorID,
			Note:        in.Reason,
			Now:         svc.now(),
		})
		if err != nil {
			return err
		}
		out = updated
		return nil
	})
	return out, err
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (svc *service) checkCapacity(c Container, projectedCBM, projectedWeight float64) error {
	overhead := 1.0 + svc.cbmOverheadPct/100
	if overhead < 1 {
		overhead = 1
	}
	if projectedCBM > c.MaxCBM*overhead {
		return domain.NewBizError(domain.ErrInsufficientStock,
			"cbm capacity exceeded for container "+c.Code)
	}
	if projectedWeight > c.MaxPayloadKG*overhead {
		return domain.NewBizError(domain.ErrInsufficientStock,
			"weight capacity exceeded for container "+c.Code)
	}
	return nil
}

func (svc *service) checkCapacityFromAggregate(ctx context.Context, tx txStore, c Container, addCBM, addWeight float64) error {
	curCBM, curWeight, err := tx.sumLinesAggregates(ctx, c.ID)
	if err != nil {
		return err
	}
	return svc.checkCapacity(c, curCBM+addCBM, curWeight+addWeight)
}

func (svc *service) flipToLoadingIfOpen(ctx context.Context, tx txStore, c Container, actor uuid.UUID) error {
	if c.Status != ContainerStatusOpen {
		return nil
	}
	_, err := tx.updateContainerStatus(ctx, updateStatusInput{
		ContainerID: c.ID,
		FromStatus:  ContainerStatusOpen,
		ToStatus:    ContainerStatusLoading,
		ActorID:     actor,
		Note:        "first line added via transfer",
		Now:         svc.now(),
	})
	return err
}

// canHoldLines reports whether a container is in a status that allows lines
// to be added or moved into/out of it.
func canHoldLines(status string) bool {
	return status == ContainerStatusOpen || status == ContainerStatusLoading
}

// orderUUIDs returns the two UUIDs sorted lexicographically — used by
// TransferLine to lock containers in a deterministic order so concurrent
// transfers between the same pair of containers cannot deadlock.
func orderUUIDs(a, b uuid.UUID) (uuid.UUID, uuid.UUID) {
	if bytes.Compare(a[:], b[:]) < 0 {
		return a, b
	}
	return b, a
}

func reasonBlank(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// roundPct keeps fill-percent values to 2 decimals so the JSON payload stays
// stable across runs (otherwise float64 division dribbles 14 digits).
func roundPct(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// ── Loading plans (#301) ────────────────────────────────────────────────────

func (svc *service) UploadLoadingPlan(ctx context.Context, in UploadLoadingPlanInput) (LoadingPlanUploadResult, error) {
	if in.ContainerID == uuid.Nil {
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	if in.CustomerID == uuid.Nil {
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrInvalidInput, "customer_id is required")
	}
	if in.UploadedBy == uuid.Nil {
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrInvalidInput, "uploaded_by is required")
	}
	if svc.skuResolver == nil {
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrPreconditionFailed, "customer SKU resolver is not wired; cannot parse loading plans")
	}

	// Container must exist and be in a status that still allows packing intent.
	c, err := svc.s.selectContainerByID(ctx, in.ContainerID)
	if err != nil {
		return LoadingPlanUploadResult{}, err
	}
	switch c.Status {
	case ContainerStatusSealed, ContainerStatusShipped, ContainerStatusCancelled:
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrInvalidTransition, "container is "+c.Status+"; loading plan can only be uploaded while OPEN or LOADING")
	}

	parsed, err := parseExcelV1(in.File)
	if err != nil {
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrInvalidInput, err.Error())
	}
	if len(parsed.RowErrors) > 0 {
		// Fail-all mirrors BR-D08; never persist a partial plan.
		return LoadingPlanUploadResult{Errors: parsed.RowErrors}, domain.NewBizError(domain.ErrInvalidInput, "loading plan has row errors")
	}
	if len(parsed.Rows) == 0 {
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrInvalidInput, "loading plan has no data rows")
	}

	// BR-D10 service-level guard. The DB partial unique index is the
	// authoritative backstop, but checking here gives the operator a
	// human 409 instead of a constraint-violation 400.
	active, err := svc.s.selectActiveLoadingPlan(ctx, in.ContainerID)
	if err == nil && active.ExcelHash == parsed.Hash && active.Status != LoadingPlanStatusSuperseded {
		return LoadingPlanUploadResult{}, domain.NewBizError(domain.ErrInvalidInput, "an active plan with the same Excel hash already exists for this container")
	} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return LoadingPlanUploadResult{}, err
	}

	// Resolve every row's customer_sku_code → sku_id. Accumulate misses
	// before bailing so the operator sees every missing mapping in one
	// upload cycle instead of fixing them one at a time.
	var rowErrs []LoadingPlanRowError
	resolved := make([]LoadingPlanLine, 0, len(parsed.Rows))
	now := svc.now().UTC()
	planID := uuid.New()
	for _, row := range parsed.Rows {
		skuID, lookupErr := svc.skuResolver.ResolveCustomerSKU(ctx, in.CustomerID, row.CustomerSKUCode)
		if lookupErr != nil {
			if errors.Is(lookupErr, domain.ErrNotFound) {
				rowErrs = append(rowErrs, LoadingPlanRowError{
					Row:     row.RowNum,
					Col:     "A",
					Code:    "UNMAPPED_SKU",
					Message: "customer_sku_code '" + row.CustomerSKUCode + "' is not mapped for this customer",
				})
				continue
			}
			return LoadingPlanUploadResult{}, lookupErr
		}
		// qty_planned_pieces is the integer quantity. v1 template uses col B
		// as integer pieces; if the customer's file uses fractional units the
		// parser already accepts the float and we round up so we never short
		// the customer.
		pieces := int(row.QtyInExcel)
		if float64(pieces) < row.QtyInExcel {
			pieces++
		}
		resolved = append(resolved, LoadingPlanLine{
			ID:               uuid.New(),
			LoadingPlanID:    planID,
			SKUID:            skuID,
			QtyPlannedPieces: pieces,
			UnitInExcel:      row.UnitInExcel,
			QtyInExcel:       row.QtyInExcel,
			CustomerSKUCode:  row.CustomerSKUCode,
			RawExcelRow:      rawRowJSON(row.Raw),
			ExcelRowNum:      row.RowNum,
			CreatedAt:        now,
		})
	}
	if len(rowErrs) > 0 {
		return LoadingPlanUploadResult{Errors: rowErrs}, domain.NewBizError(domain.ErrInvalidInput, "loading plan has unmapped SKUs")
	}

	version, err := svc.s.nextLoadingPlanVersion(ctx, in.ContainerID)
	if err != nil {
		return LoadingPlanUploadResult{}, err
	}

	plan := LoadingPlan{
		ID:           planID,
		ContainerID:  in.ContainerID,
		ExcelFileURL: in.ExcelFileURL,
		ExcelHash:    parsed.Hash,
		ParsedAt:     now,
		UploadedBy:   in.UploadedBy,
		Status:       LoadingPlanStatusParsed,
		Version:      version,
		Notes:        strings.TrimSpace(in.Notes),
		CreatedAt:    now,
	}
	if err := svc.s.insertLoadingPlanWithLines(ctx, plan, resolved); err != nil {
		return LoadingPlanUploadResult{}, err
	}
	plan.Lines = resolved

	svc.logLoadingPlanAudit(ctx, AuditLoadingPlanInput{
		Action:      AuditLPActionUploaded,
		PlanID:      plan.ID,
		ContainerID: plan.ContainerID,
		Version:     plan.Version,
		ExcelHash:   plan.ExcelHash,
		ActorID:     in.UploadedBy,
		Notes:       plan.Notes,
	})

	return LoadingPlanUploadResult{Plan: plan, Lines: resolved}, nil
}

func (svc *service) GetActiveLoadingPlan(ctx context.Context, containerID uuid.UUID) (LoadingPlan, error) {
	if containerID == uuid.Nil {
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	plan, err := svc.s.selectActiveLoadingPlan(ctx, containerID)
	if err != nil {
		return LoadingPlan{}, err
	}
	lines, err := svc.s.selectLoadingPlanLines(ctx, plan.ID)
	if err != nil {
		return LoadingPlan{}, err
	}
	plan.Lines = lines
	return plan, nil
}

func (svc *service) GetLoadingPlan(ctx context.Context, planID uuid.UUID) (LoadingPlan, error) {
	if planID == uuid.Nil {
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidInput, "plan_id is required")
	}
	plan, err := svc.s.selectLoadingPlanByID(ctx, planID)
	if err != nil {
		return LoadingPlan{}, err
	}
	lines, err := svc.s.selectLoadingPlanLines(ctx, plan.ID)
	if err != nil {
		return LoadingPlan{}, err
	}
	plan.Lines = lines
	return plan, nil
}

// DiffLoadingPlans diffs two plan ids by SKU. The "key" is sku_id so a
// customer code rename (mapping moved to a different sku) shows up as one
// removed + one added line, matching what the FE would render.
func (svc *service) DiffLoadingPlans(ctx context.Context, planID, againstID uuid.UUID) (LoadingPlanDiff, error) {
	if planID == uuid.Nil || againstID == uuid.Nil {
		return LoadingPlanDiff{}, domain.NewBizError(domain.ErrInvalidInput, "plan_id and against are required")
	}
	if planID == againstID {
		return LoadingPlanDiff{}, domain.NewBizError(domain.ErrInvalidInput, "plan_id and against must differ")
	}

	newPlan, err := svc.GetLoadingPlan(ctx, planID)
	if err != nil {
		return LoadingPlanDiff{}, err
	}
	oldPlan, err := svc.GetLoadingPlan(ctx, againstID)
	if err != nil {
		return LoadingPlanDiff{}, err
	}
	if newPlan.ContainerID != oldPlan.ContainerID {
		return LoadingPlanDiff{}, domain.NewBizError(domain.ErrInvalidInput, "diffed plans belong to different containers")
	}

	bySKU := func(lines []LoadingPlanLine) map[uuid.UUID]LoadingPlanLine {
		m := make(map[uuid.UUID]LoadingPlanLine, len(lines))
		for _, l := range lines {
			m[l.SKUID] = l
		}
		return m
	}
	oldBy := bySKU(oldPlan.Lines)
	newBy := bySKU(newPlan.Lines)

	out := LoadingPlanDiff{Against: againstID, NewPlan: planID}
	for sku, nl := range newBy {
		if ol, ok := oldBy[sku]; ok {
			if ol.QtyPlannedPieces != nl.QtyPlannedPieces {
				out.Changed = append(out.Changed, LoadingPlanLineDiff{
					SKUID:           sku,
					CustomerSKUCode: nl.CustomerSKUCode,
					OldQty:          ol.QtyPlannedPieces,
					NewQty:          nl.QtyPlannedPieces,
				})
			}
		} else {
			out.Added = append(out.Added, nl)
		}
	}
	for sku, ol := range oldBy {
		if _, ok := newBy[sku]; !ok {
			out.Removed = append(out.Removed, ol)
		}
	}
	return out, nil
}

func (svc *service) ApproveLoadingPlan(ctx context.Context, in ApproveLoadingPlanInput) (LoadingPlan, error) {
	if in.PlanID == uuid.Nil {
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidInput, "plan_id is required")
	}
	if in.ActorID == uuid.Nil {
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidInput, "actor_id is required")
	}

	// Resolve the target plan first so we know which container the BR-D12
	// SEALED guard and the BR-D11 line-count check should run against. The
	// row read is unlocked — supersedeLoadingPlanTx re-locks it for the
	// actual update so the read-then-write race is bounded.
	target, err := svc.s.selectLoadingPlanByID(ctx, in.PlanID)
	if err != nil {
		return LoadingPlan{}, err
	}

	// BR-D12: SEALED / SHIPPED containers refuse approve until an admin
	// force-unseals (path covered by ReopenContainer). Cancelled containers
	// likewise don't accept new plans — the FE should never offer the action,
	// but the BE backstop keeps the audit chain clean.
	container, err := svc.s.selectContainerByID(ctx, target.ContainerID)
	if err != nil {
		return LoadingPlan{}, err
	}
	switch container.Status {
	case ContainerStatusSealed:
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidTransition,
			"container is SEALED — force-unseal before approving a new plan")
	case ContainerStatusShipped:
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidTransition,
			"container has shipped — cannot approve another plan")
	case ContainerStatusCancelled:
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidTransition,
			"container is cancelled")
	}

	// BR-D11: when the container already has live container_lines we must
	// supersede them. Require explicit ConfirmSupersede so the FE can show
	// the confirm dialog and the operator's intent is logged.
	lineCount, err := svc.s.countContainerLines(ctx, target.ContainerID)
	if err != nil {
		// countContainerLines uses the pool; remap to a service-shaped error.
		return LoadingPlan{}, err
	}
	mustSupersede := lineCount > 0 && target.Status != LoadingPlanStatusApproved
	if mustSupersede && !in.ConfirmSupersede {
		return LoadingPlan{}, domain.NewBizError(domain.ErrPreconditionFailed,
			"container has scanned lines that will be wiped; resubmit with confirm_supersede=true")
	}

	now := svc.now().UTC()
	var (
		updated         LoadingPlan
		supersededCount int
	)
	if mustSupersede {
		updated, supersededCount, err = svc.s.supersedeLoadingPlanTx(ctx, in.PlanID, in.ActorID, now)
	} else {
		updated, err = svc.s.approveLoadingPlanTx(ctx, in.PlanID, in.ActorID, now)
	}
	if err != nil {
		return LoadingPlan{}, err
	}
	lines, err := svc.s.selectLoadingPlanLines(ctx, updated.ID)
	if err != nil {
		return LoadingPlan{}, err
	}
	updated.Lines = lines

	auditAction := AuditLPActionApproved
	if supersededCount > 0 {
		auditAction = AuditLPActionSuperseded
	}
	svc.logLoadingPlanAudit(ctx, AuditLoadingPlanInput{
		Action:      auditAction,
		PlanID:      updated.ID,
		ContainerID: updated.ContainerID,
		Version:     updated.Version,
		ExcelHash:   updated.ExcelHash,
		ActorID:     in.ActorID,
		Notes:       strings.TrimSpace(in.Notes),
	})

	// BR-D13: notify packers on the kiosk only when something actually
	// changed under their feet — first-time approve with no live lines
	// stays silent.
	if supersededCount > 0 && svc.planReloader != nil {
		if err := svc.planReloader.NotifyPlanReload(ctx, PlanReloadNotice{
			ContainerID:     updated.ContainerID,
			NewPlanID:       updated.ID,
			NewVersion:      updated.Version,
			SupersededLines: supersededCount,
			ActorID:         in.ActorID,
		}); err != nil {
			slog.Warn("plan reload notify failed",
				"plan_id", updated.ID,
				"container_id", updated.ContainerID,
				"error", err,
			)
		}
	}
	return updated, nil
}

func (svc *service) ListContainerLinesHistory(ctx context.Context, containerID uuid.UUID, planID *uuid.UUID) ([]ContainerLineHistoryEntry, error) {
	if containerID == uuid.Nil {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	if planID != nil && *planID == uuid.Nil {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "plan_id must be a valid uuid")
	}
	return svc.s.selectContainerLinesHistory(ctx, containerID, planID)
}

const (
	atRiskDefaultDays  = 7
	atRiskRedThreshold = 3 // days_to_cutoff < 3 → RED
)

func (svc *service) ListAtRisk(ctx context.Context, days int) ([]AtRiskRow, error) {
	if days <= 0 {
		days = atRiskDefaultDays
	}
	now := svc.now()
	before := now.AddDate(0, 0, days)
	rows, err := svc.s.selectAtRiskContainers(ctx, before)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		r := &rows[i]
		daysDiff := int(r.CutoffDate.Sub(now).Hours() / 24)
		r.DaysToCutoff = daysDiff
		if r.MaxCBM > 0 {
			r.FillPctCBM = (r.UsedCBM / r.MaxCBM) * 100
		}
		if daysDiff < atRiskRedThreshold {
			r.RiskLevel = RiskLevelRed
		} else {
			r.RiskLevel = RiskLevelOrange
		}
	}
	return rows, nil
}

func (svc *service) logLoadingPlanAudit(ctx context.Context, in AuditLoadingPlanInput) {
	if svc.lpAuditor == nil {
		return
	}
	if err := svc.lpAuditor.LogLoadingPlan(ctx, in); err != nil {
		slog.Warn("loading plan audit hook failed",
			"action", in.Action,
			"plan_id", in.PlanID,
			"container_id", in.ContainerID,
			"error", err,
		)
	}
}

// ── Loader assignment (#16) ─────────────────────────────────────────────────

// AssignLoader sets or clears the loader on a container (BR-D21). When the
// container already has a different loader the call is treated as a
// reassignment and writes a container_loader_log row (BR-D22). Assigning the
// same loader that is already set is a no-op that still writes an audit row so
// supervisors can see every explicit confirmation. Only planner+ may call this
// endpoint (BR-D23) — enforced by the handler middleware, not re-checked here.
func (svc *service) AssignLoader(ctx context.Context, in AssignLoaderInput) (Container, error) {
	if in.ContainerID == uuid.Nil {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	if in.AssignedBy == uuid.Nil {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput, "assigned_by is required")
	}

	c, err := svc.s.selectContainerByID(ctx, in.ContainerID)
	if err != nil {
		return Container{}, err
	}

	// BR-D22: reassignment requires a reason.
	isReassign := c.LoaderID != nil && in.LoaderID != nil && *c.LoaderID != *in.LoaderID
	if isReassign && reasonBlank(in.Reason) {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput,
			"reason is required when reassigning a loader (BR-D22)")
	}

	entry := ContainerLoaderLog{
		ID:           uuid.New(),
		ContainerID:  in.ContainerID,
		FromLoaderID: c.LoaderID,
		ToLoaderID:   in.LoaderID,
		Reason:       in.Reason,
		AssignedBy:   in.AssignedBy,
		AssignedAt:   svc.now(),
	}
	if err := svc.s.updateContainerLoader(ctx, in.ContainerID, in.LoaderID, entry); err != nil {
		return Container{}, err
	}
	c.LoaderID = in.LoaderID
	return c, nil
}

func (svc *service) ListLoaderLog(ctx context.Context, containerID uuid.UUID) ([]ContainerLoaderLog, error) {
	if containerID == uuid.Nil {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	if _, err := svc.s.selectContainerByID(ctx, containerID); err != nil {
		return nil, err
	}
	return svc.s.selectLoaderLog(ctx, containerID)
}

// ── Packing list Excel export (#18) ─────────────────────────────────────────

func (svc *service) ExportPackingList(ctx context.Context, id uuid.UUID, w io.Writer) error {
	if id == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "container id is required")
	}
	c, err := svc.s.selectContainerByID(ctx, id)
	if err != nil {
		return err
	}
	if c.Status != ContainerStatusSealed {
		return domain.NewBizError(domain.ErrPreconditionFailed,
			fmt.Sprintf("packing list only available for SEALED containers (current status: %s)", c.Status))
	}
	lines, err := svc.s.selectContainerLines(ctx, id)
	if err != nil {
		return fmt.Errorf("select container lines: %w", err)
	}
	return buildPackingListXLSX(c, lines, w)
}

func buildPackingListXLSX(c Container, lines []ContainerLine, w io.Writer) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := "Packing List"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}

	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return fmt.Errorf("bold style: %w", err)
	}

	// ── Meta block (rows 1–4) ────────────────────────────────────────────
	meta := []struct{ label, value string }{
		{"Container", c.Code},
		{"Vessel", containerVesselName(c)},
		{"Cutoff Date", containerCutoffStr(c)},
		{"Seal Date", containerSealDateStr(c)},
	}
	for i, m := range meta {
		row := fmt.Sprintf("%d", i+1)
		_ = f.SetCellStyle(sheet, "A"+row, "A"+row, bold)
		_ = f.SetCellValue(sheet, "A"+row, m.label)
		_ = f.SetCellValue(sheet, "B"+row, m.value)
	}

	// ── Column headers (row 6) ───────────────────────────────────────────
	headers := []string{"No.", "SKU Code", "SKU Name", "Qty", "CBM Total", "Weight KG"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 6)
		_ = f.SetCellStyle(sheet, cell, cell, bold)
		_ = f.SetCellValue(sheet, cell, h)
	}

	// ── Data rows (from row 7) ───────────────────────────────────────────
	for i, l := range lines {
		row := i + 7
		skuName := l.SKUName
		if skuName == "" {
			skuName = l.SKUCode
		}
		values := []any{i + 1, l.SKUCode, skuName, l.Qty, l.CBMTotal, l.WeightKGTotal}
		for col, v := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	_ = f.SetColWidth(sheet, "A", "A", 6)
	_ = f.SetColWidth(sheet, "B", "B", 16)
	_ = f.SetColWidth(sheet, "C", "C", 32)
	_ = f.SetColWidth(sheet, "D", "D", 8)
	_ = f.SetColWidth(sheet, "E", "F", 14)

	return f.Write(w)
}

func containerVesselName(c Container) string {
	// VesselName is not on Container struct — vessel info is on the vessel row.
	// cutoff_date is denormalized; vessel name is not. Return the vessel ID
	// string as a fallback when the FE has it from context.
	if c.VesselID != nil {
		return c.VesselID.String()
	}
	return ""
}

func containerCutoffStr(c Container) string {
	if c.CutoffDate == nil {
		return ""
	}
	return c.CutoffDate.Format("2006-01-02")
}

func containerSealDateStr(c Container) string {
	if c.SealedAt == nil {
		return ""
	}
	return c.SealedAt.Format("2006-01-02")
}

// ── Destination reassignment (#17) ─────────────────────────────────────────

// ChangeDestination updates destination_code/name on a container (BR-D24).
// BR-D25: SEALED/SHIPPED containers refuse the change.
// BR-D26: when destination_code changes, vessel_id + cutoff_date are cleared
// atomically so planners must rebook — a cleared booking is surfaced by
// the at-risk dashboard (no cutoff = no risk window = invisible to at-risk).
// Reason is required when the container already has a destination and the new
// code differs from the current one.
func (svc *service) ChangeDestination(ctx context.Context, in ChangeDestinationInput) (Container, error) {
	if in.ContainerID == uuid.Nil {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	if in.ActorID == uuid.Nil {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput, "actor_id is required")
	}
	if reasonBlank(in.DestinationCode) {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput, "destination_code is required")
	}

	c, err := svc.s.selectContainerByID(ctx, in.ContainerID)
	if err != nil {
		return Container{}, err
	}

	// BR-D25: sealed/shipped containers must not have destination changed.
	switch c.Status {
	case ContainerStatusSealed, ContainerStatusShipped:
		return Container{}, domain.NewBizError(domain.ErrInvalidTransition,
			"cannot change destination after container is "+c.Status+" (BR-D25)")
	}

	// BR-D26: destination change → auto-clear vessel booking.
	isReassign := c.DestinationCode != "" && c.DestinationCode != in.DestinationCode
	if isReassign && reasonBlank(in.Reason) {
		return Container{}, domain.NewBizError(domain.ErrInvalidInput,
			"reason is required when changing destination (BR-D24)")
	}
	clearVessel := isReassign && (c.VesselID != nil)

	entry := ContainerRouteChangeLog{
		ID:          uuid.New(),
		ContainerID: in.ContainerID,
		FromDC:      c.DestinationCode,
		ToDC:        in.DestinationCode,
		FromDest:    c.DestinationName,
		ToDest:      in.DestinationName,
		Reason:      in.Reason,
		ActorID:     in.ActorID,
		ChangedAt:   svc.now(),
	}
	if err := svc.s.changeDestinationTx(ctx, in.ContainerID, in.DestinationCode, in.DestinationName, clearVessel, entry); err != nil {
		return Container{}, err
	}

	c.DestinationCode = in.DestinationCode
	c.DestinationName = in.DestinationName
	if clearVessel {
		c.VesselID = nil
		c.CutoffDate = nil
	}
	return c, nil
}

func (svc *service) ListRouteLog(ctx context.Context, containerID uuid.UUID) ([]ContainerRouteChangeLog, error) {
	if containerID == uuid.Nil {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "container_id is required")
	}
	if _, err := svc.s.selectContainerByID(ctx, containerID); err != nil {
		return nil, err
	}
	return svc.s.selectRouteLog(ctx, containerID)
}
