package loading_exception

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s          store
	skuChecker SKUChecker
	soLine     SOLineChecker
	carryOver  CarryOverCreator
	audit      AuditLogger
	now        func() time.Time
	newID      func() uuid.UUID
}

// NewService wires the production stack. Optional deps (skuChecker, soLine,
// carryOver, audit) may be nil — when nil, related side effects are skipped
// or short-circuited with a domain error so a missing dep is loud rather
// than silent.
func NewService(s store, sku SKUChecker, soLine SOLineChecker, carry CarryOverCreator, audit AuditLogger) Service {
	return &service{
		s:          s,
		skuChecker: sku,
		soLine:     soLine,
		carryOver:  carry,
		audit:      audit,
		now:        time.Now,
		newID:      uuid.New,
	}
}

// Create raises a new pending exception (manual path).
func (svc *service) Create(ctx context.Context, in CreateInput) (LoadingException, error) {
	if !validType(in.ExceptionType) {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"invalid exception_type")
	}
	if in.ContainerID == uuid.Nil {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"container_id is required")
	}
	if in.Reason == "" {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"reason is required")
	}
	if in.CreatedBy == uuid.Nil {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"created_by is required")
	}
	if in.SKUID != nil && svc.skuChecker != nil {
		if _, err := svc.skuChecker.GetSKU(ctx, *in.SKUID); err != nil {
			return LoadingException{}, err
		}
	}
	row := LoadingException{
		ID:            svc.newID(),
		ContainerID:   in.ContainerID,
		LoadingPlanID: in.LoadingPlanID,
		ExceptionType: in.ExceptionType,
		SKUID:         in.SKUID,
		Qty:           in.Qty,
		Reason:        in.Reason,
		PhotoURLs:     defaultPhotoURLs(in.PhotoURLs),
		CreatedBy:     in.CreatedBy,
		CreatedAt:     svc.now().UTC(),
	}
	if err := svc.s.insert(ctx, row); err != nil {
		return LoadingException{}, err
	}
	svc.logAudit(ctx, AuditInput{
		Action:        AuditActionCreated,
		ExceptionID:   row.ID,
		ContainerID:   row.ContainerID,
		ExceptionType: row.ExceptionType,
		ActorID:       row.CreatedBy,
		Notes:         row.Reason,
	})
	return row, nil
}

// AutoCreate is the cross-module hook. Re-uses Create with a synthesized
// reason when caller did not supply one; the validation layer is identical.
func (svc *service) AutoCreate(ctx context.Context, in AutoCreateInput) (LoadingException, error) {
	reason := in.Reason
	if reason == "" {
		reason = "auto: " + in.ExceptionType
	}
	return svc.Create(ctx, CreateInput{
		ContainerID:   in.ContainerID,
		LoadingPlanID: in.LoadingPlanID,
		ExceptionType: in.ExceptionType,
		SKUID:         in.SKUID,
		Qty:           in.Qty,
		Reason:        reason,
		PhotoURLs:     in.PhotoURLs,
		CreatedBy:     in.CreatedBy,
	})
}

// Approve flips approved_by + sets resolution. Side effects per resolution
// (BR-D17 BACKORDER carry-over) run inside the same tx so the carry-over
// SO line and the exception update commit atomically.
func (svc *service) Approve(ctx context.Context, in ApproveInput) (LoadingException, error) {
	if !validResolution(in.Resolution) {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"invalid resolution")
	}
	if in.ApprovedBy == uuid.Nil {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"approved_by is required")
	}

	var out LoadingException
	err := svc.s.withTx(ctx, func(tx txStore) error {
		current, err := tx.lockForUpdate(ctx, in.ID)
		if err != nil {
			return err
		}
		if current.ApprovedBy != nil {
			return domain.NewBizError(domain.ErrInvalidTransition,
				"loading exception is already approved")
		}

		row := approveRow{
			ID:              in.ID,
			ApprovedBy:      in.ApprovedBy,
			Resolution:      in.Resolution,
			ResolutionNotes: in.ResolutionNotes,
		}

		switch in.Resolution {
		case ResolutionBackorder:
			if in.ParentSOLineID == nil {
				return domain.NewBizError(domain.ErrInvalidInput,
					"parent_so_line_id is required for BACKORDER resolution")
			}
			if svc.carryOver == nil {
				return domain.NewBizError(domain.ErrPreconditionFailed,
					"carry-over creator is not configured")
			}
			qty := 0
			if current.Qty != nil {
				qty = *current.Qty
			}
			if qty <= 0 {
				return domain.NewBizError(domain.ErrInvalidInput,
					"qty must be > 0 to BACKORDER")
			}
			newID, err := svc.carryOver.CreateCarryOver(ctx, CarryOverInput{
				ParentSOLineID: *in.ParentSOLineID,
				Qty:            qty,
				Reason:         "loading exception backorder " + current.ID.String(),
				CreatedBy:      in.ApprovedBy,
			})
			if err != nil {
				return err
			}
			row.CarryOverSOLineID = &newID

		case ResolutionSubstituteAccepted:
			if in.SubstituteSKUID == nil {
				return domain.NewBizError(domain.ErrInvalidInput,
					"substitute_sku_id is required for SUBSTITUTE_ACCEPTED resolution")
			}
			if svc.skuChecker != nil {
				if _, err := svc.skuChecker.GetSKU(ctx, *in.SubstituteSKUID); err != nil {
					return err
				}
			}
			row.SubstituteSKUID = in.SubstituteSKUID
		}

		if err := tx.approve(ctx, row); err != nil {
			return err
		}

		// Re-read inside the tx so the returned projection reflects the
		// committed values — the caller wants approved_at to be the DB-stamped
		// NOW(), not a service-clock guess.
		updated, err := tx.lockForUpdate(ctx, in.ID)
		if err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return LoadingException{}, err
	}

	svc.logAudit(ctx, AuditInput{
		Action:        AuditActionApproved,
		ExceptionID:   out.ID,
		ContainerID:   out.ContainerID,
		ExceptionType: out.ExceptionType,
		Resolution:    in.Resolution,
		ActorID:       in.ApprovedBy,
		Notes:         in.ResolutionNotes,
	})
	return out, nil
}

// Reject closes the exception without picking a resolution. The pending
// guard treats it as closed; reports can split on (resolution IS NULL) to
// surface the rejected set separately.
func (svc *service) Reject(ctx context.Context, in RejectInput) (LoadingException, error) {
	if in.Reason == "" {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"reason is required for reject")
	}
	if in.ApprovedBy == uuid.Nil {
		return LoadingException{}, domain.NewBizError(domain.ErrInvalidInput,
			"approved_by is required")
	}

	var out LoadingException
	err := svc.s.withTx(ctx, func(tx txStore) error {
		current, err := tx.lockForUpdate(ctx, in.ID)
		if err != nil {
			return err
		}
		if current.ApprovedBy != nil {
			return domain.NewBizError(domain.ErrInvalidTransition,
				"loading exception is already approved")
		}
		if err := tx.reject(ctx, rejectRow{
			ID:         in.ID,
			ApprovedBy: in.ApprovedBy,
			Reason:     in.Reason,
		}); err != nil {
			return err
		}
		updated, err := tx.lockForUpdate(ctx, in.ID)
		if err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return LoadingException{}, err
	}

	svc.logAudit(ctx, AuditInput{
		Action:        AuditActionRejected,
		ExceptionID:   out.ID,
		ContainerID:   out.ContainerID,
		ExceptionType: out.ExceptionType,
		ActorID:       in.ApprovedBy,
		Notes:         in.Reason,
	})
	return out, nil
}

func (svc *service) Get(ctx context.Context, id uuid.UUID) (LoadingException, error) {
	return svc.s.selectByID(ctx, id)
}

func (svc *service) List(ctx context.Context, containerID uuid.UUID, f ListFilter, p httpkit.CursorParams) (httpkit.CursorResult[LoadingException], error) {
	cur, err := p.Decoded()
	if err != nil {
		return httpkit.CursorResult[LoadingException]{}, domain.NewBizError(domain.ErrInvalidInput, err.Error())
	}
	rows, err := svc.s.selectByContainerKeyset(ctx, containerID, f.Status, cur, p.Limit+1)
	if err != nil {
		return httpkit.CursorResult[LoadingException]{}, err
	}
	return httpkit.NewCursorResult(rows, p.Limit, func(r LoadingException) httpkit.Cursor {
		return httpkit.Cursor{Ts: r.CreatedAt, ID: r.ID}
	}), nil
}

func (svc *service) PendingForContainer(ctx context.Context, containerID uuid.UUID) (PendingSummary, error) {
	if containerID == uuid.Nil {
		return PendingSummary{}, domain.NewBizError(domain.ErrInvalidInput,
			"container_id is required")
	}
	return svc.s.pendingByContainer(ctx, containerID)
}

func (svc *service) logAudit(ctx context.Context, in AuditInput) {
	if svc.audit == nil {
		return
	}
	if err := svc.audit.LogException(ctx, in); err != nil {
		slog.Warn("loading_exception: audit write failed",
			"action", in.Action, "exception_id", in.ExceptionID, "err", err)
	}
}

func defaultPhotoURLs(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func validType(t string) bool {
	switch t {
	case TypeShortShipped, TypeOverLoaded, TypeWrongSKU, TypeSubstitution,
		TypeDamagedAtLoading, TypeUnplannedUnit, TypeCustomerChange:
		return true
	}
	return false
}

func validResolution(r string) bool {
	switch r {
	case ResolutionBackorder, ResolutionCancelFromSO, ResolutionSubstituteAccepted,
		ResolutionWriteOff, ResolutionDeferToNext:
		return true
	}
	return false
}
