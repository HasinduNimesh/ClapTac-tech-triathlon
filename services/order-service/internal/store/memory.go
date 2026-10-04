package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

type Memory struct {
	mu          sync.Mutex
	seq         int
	byID        map[string]domain.Order
	byRef       map[string]string
	receipts    map[string]domain.Receipt
	issues      map[string][]domain.ReceiptIssue
	issueKeys   map[string]domain.ReceiptIssue
	custody     map[string][]domain.CustodyEvent
	custodyKeys map[string]domain.CustodyEvent
}

func NewMemory() *Memory {
	return &Memory{byID: map[string]domain.Order{}, byRef: map[string]string{}, receipts: map[string]domain.Receipt{}, issues: map[string][]domain.ReceiptIssue{}, issueKeys: map[string]domain.ReceiptIssue{}, custody: map[string][]domain.CustodyEvent{}, custodyKeys: map[string]domain.CustodyEvent{}}
}

func (m *Memory) AddCustodyEvent(order domain.Order, event domain.CustodyEvent) (domain.CustodyEvent, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	event.OrderID = order.ID
	if previous, ok := m.custodyKeys[event.IdempotencyKey]; ok {
		if previous.OrderID != order.ID || previous.Stage != event.Stage || previous.SealID != event.SealID || previous.Condition != event.Condition || previous.EvidenceRef != event.EvidenceRef || previous.ReceiverName != event.ReceiverName || !sameStrings(previous.SerialNumbers, event.SerialNumbers) {
			return domain.CustodyEvent{}, false, fmt.Errorf("conflict: idempotency key already used")
		}
		return previous, false, nil
	}
	items := m.custody[order.ID]
	if len(items) > 0 {
		previous := items[len(items)-1]
		if sameCustodyContent(previous, event) {
			m.custodyKeys[event.IdempotencyKey] = previous
			return previous, false, nil
		}
	}
	if len(items) > 0 && !custodyTransitionAllowed(items[len(items)-1].Stage, event.Stage) {
		return domain.CustodyEvent{}, false, fmt.Errorf("conflict: custody stage out of sequence")
	}
	if len(items) > 0 && (items[0].SealID != event.SealID || !sameStrings(items[0].SerialNumbers, event.SerialNumbers)) {
		return domain.CustodyEvent{}, false, fmt.Errorf("conflict: seal and serials must match the loaded custody record")
	}
	if len(items) == 0 && event.Stage != "LOADED" {
		return domain.CustodyEvent{}, false, fmt.Errorf("conflict: first custody stage must be LOADED")
	}
	m.seq++
	event.ID = fmt.Sprintf("custody-%d", m.seq)
	event.RecordedAt = time.Now().UTC()
	event.SerialNumbers = append([]string{}, event.SerialNumbers...)
	m.custody[order.ID] = append(items, event)
	m.custodyKeys[event.IdempotencyKey] = event
	return event, true, nil
}

func sameCustodyContent(previous, next domain.CustodyEvent) bool {
	return previous.OrderID == next.OrderID && previous.Stage == next.Stage && previous.SealID == next.SealID && previous.Condition == next.Condition && previous.EvidenceRef == next.EvidenceRef && previous.ReceiverName == next.ReceiverName && previous.RecordedBy == next.RecordedBy && sameStrings(previous.SerialNumbers, next.SerialNumbers)
}

func (m *Memory) ListCustodyEvents(orderID string) ([]domain.CustodyEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byRef[orderID]; ok {
		orderID = id
	}
	if _, ok := m.byID[orderID]; !ok {
		return nil, fmt.Errorf("not found")
	}
	items := append([]domain.CustodyEvent{}, m.custody[orderID]...)
	for i := range items {
		items[i].SerialNumbers = append([]string{}, items[i].SerialNumbers...)
	}
	return items, nil
}

func custodyTransitionAllowed(previous, next string) bool {
	allowed := map[string]string{"LOADED": "DISPATCHED", "DISPATCHED": "DELIVERED", "DELIVERED": "RECEIVED"}
	return allowed[previous] == next
}

func (m *Memory) Create(order domain.Order) (domain.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	if order.ID == "" {
		order.ID = fmt.Sprintf("id-%d", m.seq)
	}
	if order.OrderRef == "" {
		order.OrderRef = fmt.Sprintf("ORD%06d", m.seq)
	}
	m.byID[order.ID] = order
	m.byRef[order.OrderRef] = order.ID
	return order, nil
}

func (m *Memory) Get(id string) (domain.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.byID[id]
	if !ok {
		if mapped, found := m.byRef[id]; found {
			return m.byID[mapped], nil
		}
		return domain.Order{}, fmt.Errorf("not found")
	}
	return o, nil
}

func (m *Memory) List(filter domain.ListFilter) ([]domain.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.Order{}
	for _, o := range m.byID {
		if filter.OutletID != "" && o.OutletID != filter.OutletID {
			continue
		}
		if filter.Status != "" && o.Status != filter.Status {
			continue
		}
		if filter.Brand != "" && o.Brand != filter.Brand {
			continue
		}
		if filter.RequestedDeliveryDate != "" && o.RequestedDeliveryDate != filter.RequestedDeliveryDate {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

func (m *Memory) Forecast(now time.Time) (domain.Forecast, error) {
	return domain.Forecast{GeneratedAt: now.UTC(), ForecastVersion: forecastVersion, Method: "four-week mean held flat across the next ten weeks; sparse history falls back to zero", HistoryWeeks: forecastHistoryWeeks, HorizonWeeks: ForecastHorizonWeeks, DriftModelVersion: "weekly_order_shift_v1", BacktestModelVersion: "prior_four_week_order_count_ape_v1", InputDrift: []domain.ForecastInputDrift{}, Weekly: []domain.ForecastBucket{}, Capacity: []domain.ForecastCapacity{}, ServiceMinutesPerStop: 20, ServiceEstimateVersion: "fixed_20m_v1", ServiceEstimateSource: "deterministic 20-minute fallback", ServiceTimeBacktestVersion: serviceTimeBacktestVersion, ServiceTimeEvaluation: []domain.ServiceTimeEvaluation{}}, nil
}

func (m *Memory) GetImported(source, externalID string) (domain.Order, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    id, ok := m.byRef["IMPORT:"+source+"\x00"+externalID]
    if !ok { return domain.Order{}, fmt.Errorf("not found") }
    return m.byID[id], nil
}

func (m *Memory) ImportOrders(source string, orders []domain.Order) ([]domain.ImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	results := make([]domain.ImportResult, len(orders))
	pending := map[string]domain.Order{}
	for i, o := range orders {
		key := source + "\x00" + o.ExternalOrderID
		if id, ok := m.byRef["IMPORT:"+key]; ok {
			existing := m.byID[id]
			if !sameImportedOrder(existing, o) {
				return nil, fmt.Errorf("conflict: external order %s already exists with different data", o.ExternalOrderID)
			}
			results[i] = domain.ImportResult{Order: existing, Created: false}
			continue
		}
		if existing, ok := pending[key]; ok {
			if !sameImportedOrder(existing, o) {
				return nil, fmt.Errorf("conflict: duplicate external order %s has different rows", o.ExternalOrderID)
			}
			results[i] = domain.ImportResult{Order: existing, Created: false}
			continue
		}
		o.SourceSystem = source
		pending[key] = o
		results[i] = domain.ImportResult{Order: o, Created: true}
	}
	for key, o := range pending {
		m.seq++
		o.ID = fmt.Sprintf("id-%d", m.seq)
		o.OrderRef = fmt.Sprintf("ORD%06d", m.seq)
		if o.CreatedAt.IsZero() {
			o.CreatedAt = time.Now()
		}
		m.byID[o.ID] = o
		m.byRef[o.OrderRef] = o.ID
		m.byRef["IMPORT:"+key] = o.ID
		for i, input := range orders {
			if input.ExternalOrderID == o.ExternalOrderID {
				results[i].Order = o
			}
		}
	}
	return results, nil
}

func (m *Memory) GetReceipt(orderID string) (domain.Receipt, []domain.ReceiptIssue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byRef[orderID]; ok {
		orderID = id
	}
	r, ok := m.receipts[orderID]
	if !ok {
		return domain.Receipt{}, nil, fmt.Errorf("not found")
	}
	return r, append([]domain.ReceiptIssue{}, m.issues[orderID]...), nil
}

func (m *Memory) ConfirmReceipt(order domain.Order, c domain.ReceiptConfirmation) (domain.Receipt, []domain.ReceiptIssue, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.receipts[order.ID]; ok {
		if r.ReceivedUnits != c.ReceivedUnits {
			return domain.Receipt{}, nil, false, fmt.Errorf("conflict: receipt already confirmed with different quantity")
		}
		return r, append([]domain.ReceiptIssue{}, m.issues[order.ID]...), false, nil
	}
	m.seq++
	status := "confirmed"
	if c.Issue != nil {
		status = "confirmed_with_issue"
	}
	r := domain.Receipt{ID: fmt.Sprintf("receipt-%d", m.seq), OrderID: order.ID, DeliveryRunID: c.DeliveryRunID, DeliveryStopID: c.DeliveryStopID, DeliveryOutcome: c.DeliveryOutcome, ExpectedUnits: c.ExpectedUnits, ReceivedUnits: c.ReceivedUnits, Status: status, ConfirmedBy: c.ConfirmedBy, ConfirmedAt: time.Now(), Version: 1}
	m.receipts[order.ID] = r
	var added []domain.ReceiptIssue
	if c.Issue != nil {
		i := m.newIssue(r.ID, *c.Issue, c.ConfirmedBy)
		m.issues[order.ID] = append(m.issues[order.ID], i)
		m.issueKeys[i.IdempotencyKey] = i
		added = append(added, i)
	}
	return r, added, true, nil
}

func (m *Memory) AddReceiptIssue(order domain.Order, req domain.ReceiptIssueRequest, actor string) (domain.Receipt, domain.ReceiptIssue, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receipts[order.ID]
	if !ok {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, fmt.Errorf("not found")
	}
	if existing, ok := m.issueKeys[req.IdempotencyKey]; ok {
		if existing.ReceiptID != r.ID || existing.IssueType != req.IssueType || existing.AffectedUnits != req.AffectedUnits || existing.Note != req.Note {
			return domain.Receipt{}, domain.ReceiptIssue{}, false, fmt.Errorf("conflict: idempotency key already used")
		}
		return r, existing, false, nil
	}
	i := m.newIssue(r.ID, req, actor)
	m.issueKeys[i.IdempotencyKey] = i
	m.issues[order.ID] = append(m.issues[order.ID], i)
	r.Status = "confirmed_with_issue"
	r.Version++
	m.receipts[order.ID] = r
	return r, i, true, nil
}

func (m *Memory) newIssue(receiptID string, req domain.ReceiptIssueRequest, actor string) domain.ReceiptIssue {
	m.seq++
	return domain.ReceiptIssue{ID: fmt.Sprintf("issue-%d", m.seq), ReceiptID: receiptID, IssueType: req.IssueType, AffectedUnits: req.AffectedUnits, Note: req.Note, IdempotencyKey: req.IdempotencyKey, CreatedBy: actor, CreatedAt: time.Now()}
}

func (m *Memory) ListReceiptIssues() ([]domain.ReceiptIssueView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.ReceiptIssueView{}
	for orderID, issues := range m.issues {
		o, ok := m.byID[orderID]
		if !ok {
			continue
		}
		r := m.receipts[orderID]
		for _, i := range issues {
			out = append(out, domain.ReceiptIssueView{OrderRef: o.OrderRef, OutletID: o.OutletID, Receipt: r, Issue: i})
		}
	}
	return out, nil
}
