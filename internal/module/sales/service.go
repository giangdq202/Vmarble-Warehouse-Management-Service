package sales

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s          store
	skuChecker SKUChecker
	splitter   ProductionSplitter
	auditor    CustomerSKUMappingAuditLogger
	now        func() time.Time // overridable in tests
}

// NewService wires the sales module against a store and the cross-module
// dependencies it owns. splitter may be nil — endpoints other than
// SplitToPlan stay functional, and SplitToPlan returns ErrPreconditionFailed
// when called without a configured splitter.
func NewService(s store, skuChecker SKUChecker, splitter ProductionSplitter) Service {
	return &service{s: s, skuChecker: skuChecker, splitter: splitter, now: time.Now}
}

// NewServiceWithAudit wires the optional CustomerSKUMappingAuditLogger.
// Tests construct via NewService and leave auditor nil — production wires
// the salesMappingAuditAdapter from cmd/server/main.go. Audit failures are
// best-effort: logged via slog.Warn but never returned.
func NewServiceWithAudit(s store, skuChecker SKUChecker, splitter ProductionSplitter, auditor CustomerSKUMappingAuditLogger) Service {
	return &service{s: s, skuChecker: skuChecker, splitter: splitter, auditor: auditor, now: time.Now}
}

// ── Customer ─────────────────────────────────────────────────────────────────

func (svc *service) CreateCustomer(ctx context.Context, in CreateCustomerInput) (Customer, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Customer{}, domain.NewBizError(domain.ErrInvalidInput, "name is required")
	}

	code := strings.TrimSpace(in.Code)
	if code == "" {
		auto, err := svc.s.nextCustomerCode(ctx)
		if err != nil {
			return Customer{}, err
		}
		code = auto
	} else {
		exists, err := svc.s.customerCodeExists(ctx, code)
		if err != nil {
			return Customer{}, err
		}
		if exists {
			return Customer{}, domain.NewBizError(domain.ErrInvalidInput, "customer code already in use")
		}
	}

	c := Customer{
		ID:            uuid.New(),
		Code:          code,
		Name:          name,
		CountryCode:   strings.ToUpper(strings.TrimSpace(in.CountryCode)),
		Address:       strings.TrimSpace(in.Address),
		ContactPerson: strings.TrimSpace(in.ContactPerson),
		ContactPhone:  strings.TrimSpace(in.ContactPhone),
		ContactEmail:  strings.TrimSpace(in.ContactEmail),
		IsActive:      true,
		CreatedAt:     svc.now(),
	}
	if err := svc.s.insertCustomer(ctx, c); err != nil {
		return Customer{}, err
	}
	return c, nil
}

func (svc *service) ListCustomers(ctx context.Context, p httpkit.PageParams, activeOnly bool) (httpkit.PagedResult[Customer], error) {
	items, total, err := svc.s.selectCustomersPaged(ctx, p, activeOnly)
	if err != nil {
		return httpkit.PagedResult[Customer]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

func (svc *service) PatchCustomer(ctx context.Context, in PatchCustomerInput) (Customer, error) {
	c, err := svc.s.selectCustomerByID(ctx, in.ID)
	if err != nil {
		return Customer{}, err
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return Customer{}, domain.NewBizError(domain.ErrInvalidInput, "name must not be empty")
		}
		c.Name = n
	}
	if in.CountryCode != nil {
		c.CountryCode = strings.ToUpper(strings.TrimSpace(*in.CountryCode))
	}
	if in.Address != nil {
		c.Address = strings.TrimSpace(*in.Address)
	}
	if in.ContactPerson != nil {
		c.ContactPerson = strings.TrimSpace(*in.ContactPerson)
	}
	if in.ContactPhone != nil {
		c.ContactPhone = strings.TrimSpace(*in.ContactPhone)
	}
	if in.ContactEmail != nil {
		c.ContactEmail = strings.TrimSpace(*in.ContactEmail)
	}
	if in.IsActive != nil {
		c.IsActive = *in.IsActive
	}
	if err := svc.s.updateCustomer(ctx, c); err != nil {
		return Customer{}, err
	}
	return c, nil
}

// ── Sales order ──────────────────────────────────────────────────────────────

func (svc *service) CreateSO(ctx context.Context, in CreateSOInput) (SalesOrder, error) {
	if in.CustomerID == uuid.Nil {
		return SalesOrder{}, domain.NewBizError(domain.ErrInvalidInput, "customer_id is required")
	}
	if len(in.Lines) == 0 {
		return SalesOrder{}, domain.NewBizError(domain.ErrInvalidInput, "at least one line is required")
	}
	if err := validateLines(in.Lines); err != nil {
		return SalesOrder{}, err
	}
	currency := normalizeCurrency(in.Currency)
	if currency == "" {
		currency = "VND"
	}
	if err := svc.validateSKUs(ctx, in.Lines); err != nil {
		return SalesOrder{}, err
	}

	now := svc.now()
	code, err := svc.s.nextSOCode(ctx, now)
	if err != nil {
		return SalesOrder{}, err
	}

	so := SalesOrder{
		ID:               uuid.New(),
		Code:             code,
		CustomerID:       in.CustomerID,
		Incoterm:         strings.ToUpper(strings.TrimSpace(in.Incoterm)),
		PortOfLoading:    strings.TrimSpace(in.PortOfLoading),
		PortOfDischarge:  strings.TrimSpace(in.PortOfDischarge),
		Currency:         currency,
		Status:           SOStatusDraft,
		ExpectedShipDate: in.ExpectedShipDate,
		Note:             strings.TrimSpace(in.Note),
		CreatedBy:        in.CreatedBy,
		CreatedAt:        now,
	}
	if err := svc.s.insertSO(ctx, so); err != nil {
		return SalesOrder{}, err
	}

	lines := make([]SalesOrderLine, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = SalesOrderLine{
			ID:           uuid.New(),
			SalesOrderID: so.ID,
			SKUID:        l.SKUID,
			QtyOrdered:   l.QtyOrdered,
			UnitPrice:    domain.Money{Amount: l.UnitPrice.Amount, Currency: normalizeCurrency(l.UnitPrice.Currency)},
			CreatedAt:    now,
		}
	}
	if err := svc.s.insertSOLines(ctx, lines); err != nil {
		return SalesOrder{}, err
	}
	so.Lines = lines
	return so, nil
}

func (svc *service) GetSO(ctx context.Context, id uuid.UUID) (SalesOrder, error) {
	so, err := svc.s.selectSOByID(ctx, id)
	if err != nil {
		return SalesOrder{}, err
	}
	lines, err := svc.s.selectSOLinesBySOID(ctx, id)
	if err != nil {
		return SalesOrder{}, err
	}
	so.Lines = lines
	return so, nil
}

func (svc *service) ListSOs(ctx context.Context, p httpkit.PageParams, f SOListFilter) (httpkit.PagedResult[SalesOrder], error) {
	sos, total, err := svc.s.selectSOsPaged(ctx, p, f)
	if err != nil {
		return httpkit.PagedResult[SalesOrder]{}, err
	}
	for i := range sos {
		lines, err := svc.s.selectSOLinesBySOID(ctx, sos[i].ID)
		if err != nil {
			return httpkit.PagedResult[SalesOrder]{}, err
		}
		sos[i].Lines = lines
	}
	return httpkit.NewPagedResult(sos, total, p), nil
}

// PatchSO honours BR-S01: only DRAFT orders may be edited. Lines, when
// supplied, fully replace the existing set — no partial-line PATCH semantics.
func (svc *service) PatchSO(ctx context.Context, in PatchSOInput) (SalesOrder, error) {
	so, err := svc.s.selectSOByID(ctx, in.ID)
	if err != nil {
		return SalesOrder{}, err
	}
	if so.Status != SOStatusDraft {
		return SalesOrder{}, domain.NewBizError(domain.ErrInvalidTransition, "only DRAFT orders can be edited")
	}

	if in.Incoterm != nil {
		so.Incoterm = strings.ToUpper(strings.TrimSpace(*in.Incoterm))
	}
	if in.PortOfLoading != nil {
		so.PortOfLoading = strings.TrimSpace(*in.PortOfLoading)
	}
	if in.PortOfDischarge != nil {
		so.PortOfDischarge = strings.TrimSpace(*in.PortOfDischarge)
	}
	if in.Currency != nil {
		so.Currency = normalizeCurrency(*in.Currency)
	}
	if in.ClearExpectedShipDate {
		so.ExpectedShipDate = nil
	} else if in.ExpectedShipDate != nil {
		ts := *in.ExpectedShipDate
		so.ExpectedShipDate = &ts
	}
	if in.Note != nil {
		so.Note = strings.TrimSpace(*in.Note)
	}

	if err := svc.s.updateSO(ctx, so); err != nil {
		return SalesOrder{}, err
	}

	if in.Lines != nil {
		newLines := *in.Lines
		if len(newLines) == 0 {
			return SalesOrder{}, domain.NewBizError(domain.ErrInvalidInput, "at least one line is required")
		}
		if err := validateLines(newLines); err != nil {
			return SalesOrder{}, err
		}
		if err := svc.validateSKUs(ctx, newLines); err != nil {
			return SalesOrder{}, err
		}
		if err := svc.s.deleteSOLinesBySO(ctx, so.ID); err != nil {
			return SalesOrder{}, err
		}
		now := svc.now()
		built := make([]SalesOrderLine, len(newLines))
		for i, l := range newLines {
			built[i] = SalesOrderLine{
				ID:           uuid.New(),
				SalesOrderID: so.ID,
				SKUID:        l.SKUID,
				QtyOrdered:   l.QtyOrdered,
				UnitPrice:    domain.Money{Amount: l.UnitPrice.Amount, Currency: normalizeCurrency(l.UnitPrice.Currency)},
				CreatedAt:    now,
			}
		}
		if err := svc.s.insertSOLines(ctx, built); err != nil {
			return SalesOrder{}, err
		}
		so.Lines = built
	} else {
		lines, err := svc.s.selectSOLinesBySOID(ctx, so.ID)
		if err != nil {
			return SalesOrder{}, err
		}
		so.Lines = lines
	}
	return so, nil
}

// ConfirmSO transitions DRAFT → CONFIRMED. BR-S05 export validation runs here
// (not in CreateSO/PatchSO) so DRAFT orders may be saved with incomplete
// export fields and finalized later.
func (svc *service) ConfirmSO(ctx context.Context, id uuid.UUID) error {
	so, err := svc.s.selectSOByID(ctx, id)
	if err != nil {
		return err
	}
	if so.Status != SOStatusDraft {
		return domain.NewBizError(domain.ErrInvalidTransition, "only DRAFT orders can be confirmed")
	}
	if err := validateExportFields(so); err != nil {
		return err
	}
	return svc.s.updateSOStatus(ctx, id, SOStatusConfirmed)
}

// CancelSO honours BR-S04: cancellation requires every line to have
// qty_shipped == 0. Status guard rejects PARTIALLY_SHIPPED / SHIPPED on top
// of the qty check; CANCELLED is idempotent-rejected.
func (svc *service) CancelSO(ctx context.Context, in CancelSOInput) error {
	so, err := svc.s.selectSOByID(ctx, in.ID)
	if err != nil {
		return err
	}
	switch so.Status {
	case SOStatusShipped, SOStatusPartiallyShipped:
		return domain.NewBizError(domain.ErrInvalidTransition, "cannot cancel a shipped order")
	case SOStatusCancelled:
		return domain.NewBizError(domain.ErrInvalidTransition, "order already cancelled")
	}

	lines, err := svc.s.selectSOLinesBySOID(ctx, in.ID)
	if err != nil {
		return err
	}
	for _, l := range lines {
		if l.QtyShipped > 0 {
			return domain.NewBizError(domain.ErrInvalidTransition,
				"cannot cancel: line "+l.ID.String()+" already has shipped quantity")
		}
	}
	return svc.s.updateSOStatus(ctx, in.ID, SOStatusCancelled)
}

// SplitToPlan creates a production plan + work orders for the given line
// allocations and bumps qty_planned atomically. CONFIRMED orders auto-flip
// to IN_PRODUCTION on the first successful split (BR-S07).
//
// Failure modes the caller must understand:
//   - validation / qty overflow → no production work happens, no qty_planned mutation
//   - production split fails    → no qty_planned mutation
//   - production split succeeds but the sales tx fails → WOs exist with stale
//     qty_planned. The error wraps ErrPreconditionFailed and lists the orphaned
//     plan code so the planner can reconcile.
func (svc *service) SplitToPlan(ctx context.Context, in SplitToPlanInput) (SplitToPlanResult, error) {
	if svc.splitter == nil {
		return SplitToPlanResult{}, domain.NewBizError(domain.ErrPreconditionFailed, "production splitter is not configured")
	}
	if len(in.Allocations) == 0 {
		return SplitToPlanResult{}, domain.NewBizError(domain.ErrInvalidInput, "at least one allocation is required")
	}
	for _, a := range in.Allocations {
		if a.Quantity <= 0 {
			return SplitToPlanResult{}, domain.NewBizError(domain.ErrInvalidInput, "allocation quantity must be greater than 0")
		}
	}

	// Phase 1: lock the SO and lines, validate qty caps. Holding the lock
	// across the production call would tie up the connection; releasing it
	// before the cross-module call is safe because the DB CHECK still
	// enforces qty_planned <= qty_ordered as a backstop.
	var (
		so          SalesOrder
		lockedLines []SalesOrderLine
	)
	lineIDs := make([]uuid.UUID, len(in.Allocations))
	for i, a := range in.Allocations {
		lineIDs[i] = a.SOLineID
	}
	err := svc.s.withTx(ctx, func(tx txStore) error {
		s, err := tx.lockSOForUpdate(ctx, in.SalesOrderID)
		if err != nil {
			return err
		}
		switch s.Status {
		case SOStatusConfirmed, SOStatusInProduction, SOStatusPartiallyShipped:
			// continue
		default:
			return domain.NewBizError(domain.ErrInvalidTransition,
				"sales order must be CONFIRMED or later to split, got "+s.Status)
		}
		ls, err := tx.lockAndReadSOLines(ctx, lineIDs)
		if err != nil {
			return err
		}
		// Validate every alloc: line belongs to this SO and qty_planned + delta ≤ qty_ordered.
		for i, a := range in.Allocations {
			l := ls[i]
			if l.SalesOrderID != in.SalesOrderID {
				return domain.NewBizError(domain.ErrInvalidInput,
					"line "+l.ID.String()+" does not belong to this sales order")
			}
			if l.QtyPlanned+a.Quantity > l.QtyOrdered {
				return domain.NewBizError(domain.ErrInvalidInput,
					"allocation exceeds remaining qty for line "+l.ID.String())
			}
		}
		so = s
		lockedLines = ls
		return nil
	})
	if err != nil {
		return SplitToPlanResult{}, err
	}

	// Phase 2: hand off to production. Adapter creates plan + WOs in a single
	// planning-side tx. If this fails, no qty_planned mutation has happened.
	items := make([]CreatePlanWOItem, len(in.Allocations))
	for i, a := range in.Allocations {
		items[i] = CreatePlanWOItem{
			SOLineID: a.SOLineID,
			SKUID:    lockedLines[i].SKUID,
			Quantity: a.Quantity,
		}
	}
	deadline := pickSplitDeadline(in.Deadline, so.ExpectedShipDate)
	pres, err := svc.splitter.CreatePlanWithWOs(ctx, CreatePlanWithWOsRequest{
		SalesOrderID: in.SalesOrderID,
		Deadline:     deadline,
		ActorID:      in.ActorID,
		Items:        items,
	})
	if err != nil {
		return SplitToPlanResult{}, err
	}

	// Phase 3: bump qty_planned and (optionally) flip CONFIRMED → IN_PRODUCTION.
	// If this tx fails, the plan + WOs from Phase 2 outlive the qty mutation —
	// flagged in the error so the operator can reconcile.
	if err := svc.s.withTx(ctx, func(tx txStore) error {
		for _, a := range in.Allocations {
			if err := tx.incrementQtyPlanned(ctx, a.SOLineID, a.Quantity); err != nil {
				return err
			}
		}
		if so.Status == SOStatusConfirmed {
			if _, err := tx.updateStatusIfCurrent(ctx, in.SalesOrderID,
				[]string{SOStatusConfirmed}, SOStatusInProduction); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return SplitToPlanResult{}, domain.NewBizError(domain.ErrPreconditionFailed,
			"split partially applied: plan "+pres.PlanCode+" was created but qty_planned failed to update — manual reconcile required ("+err.Error()+")")
	}

	return SplitToPlanResult(pres), nil
}

// ── Cross-module hooks (delivery → sales) ────────────────────────────────────

// GetSOLine returns one line + its parent SO. Used by delivery.AddLine to
// validate that the line belongs to a shippable SO and that the chosen SKU
// matches.
func (svc *service) GetSOLine(ctx context.Context, soLineID uuid.UUID) (SalesOrderLine, SalesOrder, error) {
	return svc.s.selectSOLineByID(ctx, soLineID)
}

// RecordShipmentTx delegates to the store's transactional shipment writer.
// The pgx.Tx is owned by the caller (delivery.Seal) so the entire seal —
// container.status flip, qty_shipped bump, SO status recompute — runs in a
// single transaction. See deps.go in delivery for the full contract.
func (svc *service) RecordShipmentTx(ctx context.Context, tx pgx.Tx, items []ShipmentItemInput) error {
	return svc.s.recordShipmentTx(ctx, tx, items)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func validateLines(lines []CreateSOLineInput) error {
	seen := make(map[uuid.UUID]bool, len(lines))
	for _, l := range lines {
		if l.SKUID == uuid.Nil {
			return domain.NewBizError(domain.ErrInvalidInput, "sku_id is required on every line")
		}
		if l.QtyOrdered <= 0 {
			return domain.NewBizError(domain.ErrInvalidInput, "qty_ordered must be greater than 0")
		}
		if l.UnitPrice.Amount < 0 {
			return domain.NewBizError(domain.ErrInvalidInput, "unit price amount must be non-negative")
		}
		if normalizeCurrency(l.UnitPrice.Currency) == "" {
			return domain.NewBizError(domain.ErrInvalidInput, "unit price currency is required")
		}
		if seen[l.SKUID] {
			return domain.NewBizError(domain.ErrInvalidInput, "duplicate sku in lines: "+l.SKUID.String())
		}
		seen[l.SKUID] = true
	}
	return nil
}

func (svc *service) validateSKUs(ctx context.Context, lines []CreateSOLineInput) error {
	if svc.skuChecker == nil {
		return nil
	}
	for _, l := range lines {
		if _, err := svc.skuChecker.GetSKU(ctx, l.SKUID); err != nil {
			return err
		}
	}
	return nil
}

// validateExportFields enforces BR-S05 at confirm time: any non-VN customer
// or non-VND currency must come with the matching incoterm/port metadata.
// Domestic VND orders skip the check.
func validateExportFields(so SalesOrder) error {
	isExport := so.Currency != "VND" || (so.CustomerCountry != "" && so.CustomerCountry != "VN")
	if !isExport {
		return nil
	}
	if so.Incoterm == "" || so.PortOfLoading == "" || so.PortOfDischarge == "" {
		return domain.NewBizError(domain.ErrInvalidInput,
			"export orders require incoterm, port_of_loading, and port_of_discharge")
	}
	return nil
}

func normalizeCurrency(c string) string {
	return strings.ToUpper(strings.TrimSpace(c))
}

// pickSplitDeadline returns the override when provided, else inherits the
// SO's expected_ship_date, else nil. Planning treats nil as "no deadline".
func pickSplitDeadline(override time.Time, soShip *time.Time) *time.Time {
	if !override.IsZero() {
		t := override
		return &t
	}
	if soShip != nil {
		t := *soShip
		return &t
	}
	return nil
}

// ── Customer SKU mappings (#304) ─────────────────────────────────────────────

// validateMappingCode normalises the customer-supplied SKU code: trims spaces,
// rejects empty. Length cap is 256 chars to match a sane Excel cell.
func validateMappingCode(code string) (string, error) {
	c := strings.TrimSpace(code)
	if c == "" {
		return "", domain.NewBizError(domain.ErrInvalidInput, "customer_sku_code is required")
	}
	if len(c) > 256 {
		return "", domain.NewBizError(domain.ErrInvalidInput, "customer_sku_code must be at most 256 characters")
	}
	return c, nil
}

// requireSKU rejects nil/zero IDs and bounces existence checks through the
// catalog SKUChecker. SKUChecker also handles the "deactivated" semantics —
// if the catalog later wants to reject mappings to inactive SKUs, the gate
// belongs there, not here.
func (svc *service) requireSKU(ctx context.Context, skuID uuid.UUID) error {
	if skuID == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "sku_id is required")
	}
	if svc.skuChecker == nil {
		// Test wiring without a SKU checker: defer to the FK constraint at
		// insert time. Production always wires one in main.go.
		return nil
	}
	if _, err := svc.skuChecker.GetSKU(ctx, skuID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NewBizError(domain.ErrInvalidInput, "sku_id does not exist")
		}
		return err
	}
	return nil
}

func (svc *service) CreateCustomerSKUMapping(ctx context.Context, in CreateCustomerSKUMappingInput) (CustomerSKUMapping, error) {
	if in.CustomerID == uuid.Nil {
		return CustomerSKUMapping{}, domain.NewBizError(domain.ErrInvalidInput, "customer_id is required")
	}
	code, err := validateMappingCode(in.CustomerSKUCode)
	if err != nil {
		return CustomerSKUMapping{}, err
	}
	if err := svc.requireSKU(ctx, in.SKUID); err != nil {
		return CustomerSKUMapping{}, err
	}

	now := svc.now().UTC()
	var createdBy *uuid.UUID
	if in.ActorID != uuid.Nil {
		id := in.ActorID
		createdBy = &id
	}

	m := CustomerSKUMapping{
		CustomerID:      in.CustomerID,
		CustomerSKUCode: code,
		SKUID:           in.SKUID,
		Notes:           strings.TrimSpace(in.Notes),
		CreatedBy:       createdBy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := svc.s.insertCustomerSKUMapping(ctx, m); err != nil {
		return CustomerSKUMapping{}, err
	}
	svc.logMappingAudit(ctx, AuditCustomerSKUMappingInput{
		Action:          AuditCSMActionCreated,
		CustomerID:      m.CustomerID,
		CustomerSKUCode: m.CustomerSKUCode,
		SKUID:           m.SKUID,
		ActorID:         in.ActorID,
		Notes:           m.Notes,
	})
	return m, nil
}

func (svc *service) ListCustomerSKUMappings(ctx context.Context, p httpkit.PageParams, f CustomerSKUMappingFilter) (httpkit.PagedResult[CustomerSKUMapping], error) {
	rows, total, err := svc.s.selectCustomerSKUMappingsPaged(ctx, p, f)
	if err != nil {
		return httpkit.PagedResult[CustomerSKUMapping]{}, err
	}
	return httpkit.NewPagedResult(rows, total, p), nil
}

// GetCustomerSKUMapping resolves one (customer_id, customer_sku_code) pair to
// its internal sku_id. Used by the delivery Excel parser (#301) — returns
// ErrNotFound when the customer has not mapped that code yet so the parser
// can emit UNMAPPED_SKU.
func (svc *service) GetCustomerSKUMapping(ctx context.Context, customerID uuid.UUID, code string) (CustomerSKUMapping, error) {
	if customerID == uuid.Nil {
		return CustomerSKUMapping{}, domain.NewBizError(domain.ErrInvalidInput, "customer_id is required")
	}
	trimmed, err := validateMappingCode(code)
	if err != nil {
		return CustomerSKUMapping{}, err
	}
	return svc.s.selectCustomerSKUMappingByPK(ctx, customerID, trimmed)
}

func (svc *service) PatchCustomerSKUMapping(ctx context.Context, in PatchCustomerSKUMappingInput) (CustomerSKUMapping, error) {
	if in.CustomerID == uuid.Nil {
		return CustomerSKUMapping{}, domain.NewBizError(domain.ErrInvalidInput, "customer_id is required")
	}
	code, err := validateMappingCode(in.CustomerSKUCode)
	if err != nil {
		return CustomerSKUMapping{}, err
	}

	current, err := svc.s.selectCustomerSKUMappingByPK(ctx, in.CustomerID, code)
	if err != nil {
		return CustomerSKUMapping{}, err
	}

	updated := current
	skuChanged := false
	if in.SKUID != nil && *in.SKUID != current.SKUID {
		if err := svc.requireSKU(ctx, *in.SKUID); err != nil {
			return CustomerSKUMapping{}, err
		}
		updated.SKUID = *in.SKUID
		skuChanged = true
	}
	if in.Notes != nil {
		updated.Notes = strings.TrimSpace(*in.Notes)
	}
	updated.UpdatedAt = svc.now().UTC()

	if err := svc.s.updateCustomerSKUMapping(ctx, updated); err != nil {
		return CustomerSKUMapping{}, err
	}

	auditInput := AuditCustomerSKUMappingInput{
		Action:          AuditCSMActionUpdated,
		CustomerID:      updated.CustomerID,
		CustomerSKUCode: updated.CustomerSKUCode,
		SKUID:           updated.SKUID,
		ActorID:         in.ActorID,
		Notes:           updated.Notes,
	}
	if skuChanged {
		prev := current.SKUID
		auditInput.PreviousSKUID = &prev
	}
	svc.logMappingAudit(ctx, auditInput)
	return updated, nil
}

func (svc *service) DeleteCustomerSKUMapping(ctx context.Context, in DeleteCustomerSKUMappingInput) error {
	if in.CustomerID == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "customer_id is required")
	}
	code, err := validateMappingCode(in.CustomerSKUCode)
	if err != nil {
		return err
	}

	// Capture the SKU id before deletion so the audit row can record what
	// was removed. ErrNotFound is returned to the caller untouched.
	current, err := svc.s.selectCustomerSKUMappingByPK(ctx, in.CustomerID, code)
	if err != nil {
		return err
	}
	if err := svc.s.deleteCustomerSKUMapping(ctx, in.CustomerID, code); err != nil {
		return err
	}
	svc.logMappingAudit(ctx, AuditCustomerSKUMappingInput{
		Action:          AuditCSMActionDeleted,
		CustomerID:      current.CustomerID,
		CustomerSKUCode: current.CustomerSKUCode,
		SKUID:           current.SKUID,
		ActorID:         in.ActorID,
	})
	return nil
}

// BulkImportCustomerSKUMappings validates every row first, then persists the
// batch in a single tx. Any row error aborts the whole import (fail-all,
// matches BR-D08). The response carries an ordered slice of row errors so
// the FE can render a per-row toast pointing at the offending CSV line.
func (svc *service) BulkImportCustomerSKUMappings(ctx context.Context, in BulkImportCustomerSKUMappingsInput) (BulkImportResult, error) {
	if in.CustomerID == uuid.Nil {
		return BulkImportResult{}, domain.NewBizError(domain.ErrInvalidInput, "customer_id is required")
	}
	if len(in.Rows) == 0 {
		return BulkImportResult{}, domain.NewBizError(domain.ErrInvalidInput, "at least one row is required")
	}

	now := svc.now().UTC()
	var actorID *uuid.UUID
	if in.ActorID != uuid.Nil {
		id := in.ActorID
		actorID = &id
	}

	var errs []BulkImportRowError
	seenCodes := make(map[string]int, len(in.Rows))
	prepared := make([]CustomerSKUMapping, 0, len(in.Rows))

	for i, raw := range in.Rows {
		rowNum := i + 1
		code, err := validateMappingCode(raw.CustomerSKUCode)
		if err != nil {
			errs = append(errs, BulkImportRowError{Row: rowNum, Code: "INVALID_CODE", Message: err.Error()})
			continue
		}
		if firstSeen, dup := seenCodes[code]; dup {
			errs = append(errs, BulkImportRowError{
				Row:     rowNum,
				Code:    "DUPLICATE_IN_BATCH",
				Message: "customer_sku_code already appeared at row " + itoa(firstSeen),
			})
			continue
		}
		seenCodes[code] = rowNum
		if err := svc.requireSKU(ctx, raw.SKUID); err != nil {
			errs = append(errs, BulkImportRowError{Row: rowNum, Code: "INVALID_SKU", Message: err.Error()})
			continue
		}
		prepared = append(prepared, CustomerSKUMapping{
			CustomerID:      in.CustomerID,
			CustomerSKUCode: code,
			SKUID:           raw.SKUID,
			Notes:           strings.TrimSpace(raw.Notes),
			CreatedBy:       actorID,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}

	if len(errs) > 0 {
		return BulkImportResult{Errors: errs}, domain.NewBizError(domain.ErrInvalidInput, "bulk import has row errors")
	}

	if err := svc.s.bulkInsertCustomerSKUMappings(ctx, prepared); err != nil {
		return BulkImportResult{}, err
	}
	svc.logMappingAudit(ctx, AuditCustomerSKUMappingInput{
		Action:       AuditCSMActionBulkImported,
		CustomerID:   in.CustomerID,
		RowsImported: len(prepared),
		ActorID:      in.ActorID,
	})
	return BulkImportResult{Inserted: len(prepared)}, nil
}

func (svc *service) logMappingAudit(ctx context.Context, in AuditCustomerSKUMappingInput) {
	if svc.auditor == nil {
		return
	}
	if err := svc.auditor.LogCustomerSKUMapping(ctx, in); err != nil {
		slog.Warn("audit log for customer_sku_mapping failed",
			"action", in.Action,
			"customer_id", in.CustomerID,
			"customer_sku_code", in.CustomerSKUCode,
			"err", err,
		)
	}
}

// itoa wraps strconv.Itoa without taking a strconv import — kept tiny because
// the bulk path is the only caller and we want to avoid a new import line in
// service.go.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	digits := make([]byte, 0, 12)
	for i > 0 {
		digits = append(digits, byte('0'+i%10))
		i /= 10
	}
	if negative {
		digits = append(digits, '-')
	}
	for l, r := 0, len(digits)-1; l < r; l, r = l+1, r-1 {
		digits[l], digits[r] = digits[r], digits[l]
	}
	return string(digits)
}
