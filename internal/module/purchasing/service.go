package purchasing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	st store
	mc MaterialChecker
	sr StockReceiver
}

func NewService(st store, mc MaterialChecker, sr StockReceiver) Service {
	return &service{st: st, mc: mc, sr: sr}
}

func (s *service) CreatePO(ctx context.Context, in CreatePOInput) (PurchaseOrder, error) {
	if in.Code == "" {
		return PurchaseOrder{}, domain.NewBizError(domain.ErrInvalidInput, "code is required")
	}
	if in.MaterialID == uuid.Nil {
		return PurchaseOrder{}, domain.NewBizError(domain.ErrInvalidInput, "material_id is required")
	}
	if _, err := s.mc.GetMaterial(ctx, in.MaterialID); err != nil {
		return PurchaseOrder{}, err
	}

	po := PurchaseOrder{
		ID:         uuid.New(),
		Code:       in.Code,
		MaterialID: in.MaterialID,
		Supplier:   in.Supplier,
		Status:     StatusDraft,
		Note:       in.Note,
		CreatedBy:  in.CreatedBy,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.st.insertPO(ctx, po); err != nil {
		return PurchaseOrder{}, err
	}
	po.Items = []POItem{}
	return po, nil
}

func (s *service) GetPO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error) {
	return s.st.selectPOByID(ctx, id)
}

func (s *service) ListPOs(ctx context.Context, p httpkit.PageParams, f POListFilter) (httpkit.PagedResult[PurchaseOrder], error) {
	pos, total, err := s.st.selectPOsPaged(ctx, p, f)
	if err != nil {
		return httpkit.PagedResult[PurchaseOrder]{}, err
	}
	if pos == nil {
		pos = []PurchaseOrder{}
	}
	return httpkit.NewPagedResult(pos, total, p), nil
}

func (s *service) AddItem(ctx context.Context, in AddPOItemInput) (POItem, error) {
	if in.Quantity <= 0 {
		return POItem{}, domain.NewBizError(domain.ErrInvalidInput, "quantity must be positive")
	}
	if in.LengthMM <= 0 || in.WidthMM <= 0 {
		return POItem{}, domain.NewBizError(domain.ErrInvalidInput, "dimensions must be positive")
	}
	if in.UnitCost.Amount < 0 {
		return POItem{}, domain.NewBizError(domain.ErrInvalidInput, "unit_cost cannot be negative")
	}

	po, err := s.st.selectPOByID(ctx, in.POID)
	if err != nil {
		return POItem{}, err
	}
	if po.Status != StatusDraft {
		return POItem{}, domain.NewBizError(domain.ErrPreconditionFailed, "items can only be added to DRAFT purchase orders")
	}

	item := POItem{
		ID:        uuid.New(),
		POID:      in.POID,
		Quantity:  in.Quantity,
		LengthMM:  in.LengthMM,
		WidthMM:   in.WidthMM,
		UnitCost:  in.UnitCost,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.st.insertPOItem(ctx, item); err != nil {
		return POItem{}, err
	}
	return item, nil
}

func (s *service) RemoveItem(ctx context.Context, poID, itemID uuid.UUID) error {
	po, err := s.st.selectPOByID(ctx, poID)
	if err != nil {
		return err
	}
	if po.Status != StatusDraft {
		return domain.NewBizError(domain.ErrPreconditionFailed, "items can only be removed from DRAFT purchase orders")
	}
	return s.st.deletePOItem(ctx, poID, itemID)
}

func (s *service) OrderPO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error) {
	po, err := s.st.selectPOByID(ctx, id)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if po.Status != StatusDraft {
		return PurchaseOrder{}, domain.NewBizError(domain.ErrInvalidTransition, "only DRAFT purchase orders can be ordered")
	}
	if len(po.Items) == 0 {
		return PurchaseOrder{}, domain.NewBizError(domain.ErrPreconditionFailed, "purchase order must have at least one item before ordering")
	}

	now := time.Now().UTC()
	if err := s.st.updatePOStatus(ctx, id, StatusOrdered, &now); err != nil {
		return PurchaseOrder{}, err
	}
	po.Status = StatusOrdered
	po.OrderedAt = &now
	return po, nil
}

func (s *service) ReceivePO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error) {
	po, err := s.st.selectPOByID(ctx, id)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if po.Status != StatusOrdered {
		return PurchaseOrder{}, domain.NewBizError(domain.ErrInvalidTransition, "only ORDERED purchase orders can be received")
	}

	for _, item := range po.Items {
		lotID, err := s.sr.ReceiveStock(ctx, ReceiveStockInput{
			MaterialID:  po.MaterialID,
			LengthMM:    item.LengthMM,
			WidthMM:     item.WidthMM,
			UnitCost:    item.UnitCost,
			Quantity:    item.Quantity,
			SupplierRef: po.Code,
		})
		if err != nil {
			return PurchaseOrder{}, err
		}
		if err := s.st.linkItemToLot(ctx, item.ID, lotID); err != nil {
			return PurchaseOrder{}, err
		}
	}

	now := time.Now().UTC()
	if err := s.st.updatePOStatus(ctx, id, StatusReceived, &now); err != nil {
		return PurchaseOrder{}, err
	}
	po.Status = StatusReceived
	po.ReceivedAt = &now
	return po, nil
}

func (s *service) CancelPO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error) {
	po, err := s.st.selectPOByID(ctx, id)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if po.Status == StatusReceived || po.Status == StatusCancelled {
		return PurchaseOrder{}, domain.NewBizError(domain.ErrInvalidTransition, "cannot cancel a RECEIVED or already CANCELLED purchase order")
	}

	if err := s.st.updatePOStatus(ctx, id, StatusCancelled, nil); err != nil {
		return PurchaseOrder{}, err
	}
	po.Status = StatusCancelled
	return po, nil
}
