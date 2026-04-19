package costing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	st   store
	wor  WorkOrderReader
	cdr  CuttingDataReader
	conr ConsumptionDataReader
}

func NewService(st store, wor WorkOrderReader, cdr CuttingDataReader, conr ConsumptionDataReader) Service {
	return &service{st: st, wor: wor, cdr: cdr, conr: conr}
}

func (s *service) ComputeCost(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error) {
	wo, err := s.wor.GetWorkOrder(ctx, workOrderID)
	if err != nil {
		return CostingRecord{}, err
	}
	if wo.Status != domain.WOCompleted {
		return CostingRecord{}, domain.NewBizError(domain.ErrPreconditionFailed, "work order must be completed before costing")
	}

	cuttingData, err := s.cdr.GetCuttingDataForWO(ctx, workOrderID)
	if err != nil {
		return CostingRecord{}, err
	}

	var materialCost domain.Money
	for _, cd := range cuttingData {
		if cd.SheetAreaMM2 <= 0 {
			continue
		}
		allocated := cd.SheetCost.Scale(cd.UsedAreaMM2, cd.SheetAreaMM2)
		materialCost = materialCost.Add(allocated)
	}

	auxiliaryCost, err := s.conr.GetConsumptionCostForWO(ctx, workOrderID)
	if err != nil {
		return CostingRecord{}, err
	}

	laborCost := domain.VND(0)
	totalCost := materialCost.Add(auxiliaryCost).Add(laborCost)

	record := CostingRecord{
		WorkOrderID:   workOrderID,
		SKUID:         wo.SKUID,
		MaterialCost:  materialCost,
		AuxiliaryCost: auxiliaryCost,
		LaborCost:     laborCost,
		TotalCost:     totalCost,
		Finalized:     false,
		CreatedAt:     time.Now().UTC(),
	}

	existing, err := s.st.selectCostingRecordByWO(ctx, workOrderID)
	if err == nil {
		if existing.Finalized {
			return CostingRecord{}, domain.NewBizError(domain.ErrAlreadyFinalized, "costing record already finalized, create an adjustment instead")
		}
		record.ID = existing.ID
		record.CreatedAt = existing.CreatedAt
		if err := s.st.updateCostingRecord(ctx, record); err != nil {
			return CostingRecord{}, err
		}
		return record, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return CostingRecord{}, err
	}

	record.ID = uuid.New()
	if err := s.st.insertCostingRecord(ctx, record); err != nil {
		return CostingRecord{}, err
	}
	return record, nil
}

func (s *service) FinalizeCost(ctx context.Context, workOrderID uuid.UUID) error {
	return s.st.finalizeCostingRecord(ctx, workOrderID)
}

func (s *service) GetCostingRecord(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error) {
	return s.st.selectCostingRecordByWO(ctx, workOrderID)
}

func (s *service) ListCostingRecords(ctx context.Context, p httpkit.PageParams, finalized *bool) (httpkit.PagedResult[CostingRecord], error) {
	records, total, err := s.st.selectCostingRecordsPaged(ctx, p, finalized)
	if err != nil {
		return httpkit.PagedResult[CostingRecord]{}, err
	}
	return httpkit.NewPagedResult(records, total, p), nil
}
