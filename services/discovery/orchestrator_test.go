package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mockStore struct {
	mu      sync.Mutex
	scans   map[uuid.UUID]*ScanRecord
	results []*ResultRecord
}

func (m *mockStore) CreateScan(ctx context.Context, scan *ScanRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scans == nil {
		m.scans = make(map[uuid.UUID]*ScanRecord)
	}
	m.scans[scan.ID] = scan
	return nil
}

func (m *mockStore) UpdateScanStatus(ctx context.Context, id uuid.UUID, status string, totalFound int, errMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.scans[id]; ok {
		s.Status = status
		s.TotalFound = totalFound
		s.Error = errMsg
	}
	return nil
}

func (m *mockStore) GetScan(ctx context.Context, id uuid.UUID) (*ScanRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scans[id], nil
}

func (m *mockStore) GetScans(ctx context.Context, siteID *uuid.UUID, page, perPage int) ([]ScanRecord, int, error) {
	return nil, 0, nil
}

func (m *mockStore) InsertResult(ctx context.Context, result *ResultRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = append(m.results, result)
	return nil
}

func (m *mockStore) GetResults(ctx context.Context, scanID uuid.UUID, page, perPage int, queryFilter string) ([]ResultRecord, int, error) {
	return nil, 0, nil
}

func (m *mockStore) MarkImported(ctx context.Context, resultIDs []uuid.UUID) error { return nil }

func (m *mockStore) CheckAlreadyInDB(ctx context.Context, xaddr string) (bool, error) { return false, nil }

func (m *mockStore) Close() error { return nil }

type mockScanner struct {
	name    string
	results []ScanResult
}

func (m *mockScanner) Name() string { return m.name }

func (m *mockScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
	ch := make(chan ScanResult)
	go func() {
		defer close(ch)
		for _, r := range m.results {
			select {
			case <-ctx.Done():
				return
			case ch <- r:
			}
		}
	}()
	return ch, nil
}

func TestOrchestrator_StartAndComplete(t *testing.T) {
	store := &mockStore{}
	scanners := map[string]Scanner{
		"mock": &mockScanner{
			name: "mock",
			results: []ScanResult{
				{XAddr: "http://10.0.0.1/onvif", IP: "10.0.0.1", Capabilities: make(CapabilitySet)},
				{XAddr: "http://10.0.0.2/onvif", IP: "10.0.0.2", Capabilities: make(CapabilitySet)},
			},
		},
	}
	o := NewScanOrchestrator(store, scanners, testLogger())
	sid1 := uuid.New()
	scan, err := o.StartScan(context.Background(), ScanRequest{
		SiteID: &sid1, Methods: []string{"mock"}, Subnets: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if scan.Status != "running" {
		t.Errorf("expected running, got %s", scan.Status)
	}

	time.Sleep(500 * time.Millisecond)

	s, _ := store.GetScan(context.Background(), scan.ID)
	if s.Status != "completed" {
		t.Errorf("expected completed, got %s", s.Status)
	}
}

func TestOrchestrator_Deduplication(t *testing.T) {
	store := &mockStore{}
	scanners := map[string]Scanner{
		"mock": &mockScanner{
			name: "mock",
			results: []ScanResult{
				{XAddr: "http://10.0.0.1/onvif", IP: "10.0.0.1", Capabilities: make(CapabilitySet)},
				{XAddr: "http://10.0.0.1/onvif", IP: "10.0.0.1", Capabilities: make(CapabilitySet)},
				{XAddr: "http://10.0.0.2/onvif", IP: "10.0.0.2", Capabilities: make(CapabilitySet)},
			},
		},
	}
	o := NewScanOrchestrator(store, scanners, testLogger())
	sid2 := uuid.New()
	o.StartScan(context.Background(), ScanRequest{
		SiteID: &sid2, Methods: []string{"mock"}, Subnets: []string{"local"},
	})
	time.Sleep(500 * time.Millisecond)

	store.mu.Lock()
	count := len(store.results)
	store.mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 unique results (dedup), got %d", count)
	}
}

func TestOrchestrator_Cancellation(t *testing.T) {
	store := &mockStore{}
	scanners := map[string]Scanner{
		"slow": &mockScanner{name: "slow", results: nil},
	}
	o := NewScanOrchestrator(store, scanners, testLogger())
	sid3 := uuid.New()
	scan, _ := o.StartScan(context.Background(), ScanRequest{
		SiteID: &sid3, Methods: []string{"slow"}, Subnets: []string{"local"},
	})
	if err := o.CancelScan(context.Background(), scan.ID); err != nil {
		t.Fatal(err)
	}
	s, _ := store.GetScan(context.Background(), scan.ID)
	if s.Status != "cancelled" {
		t.Errorf("expected cancelled, got %s", s.Status)
	}
}
