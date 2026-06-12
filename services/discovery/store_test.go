package main

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func newTestStore(t *testing.T) *ResultStore {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL not set, skipping")
	}
	db, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	db.MustExec(`CREATE TABLE IF NOT EXISTS discovery_scans (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        site_id UUID,
        status TEXT NOT NULL DEFAULT 'pending',
        methods TEXT[] NOT NULL,
        subnets TEXT[],
        ports INT[] DEFAULT '{80,554,8080}',
        total_found INT DEFAULT 0,
        error TEXT,
        started_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        created_by UUID,
        created_at TIMESTAMPTZ DEFAULT NOW()
    )`)
	db.MustExec(`CREATE TABLE IF NOT EXISTS discovery_results (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        scan_id UUID REFERENCES discovery_scans(id) ON DELETE CASCADE,
        site_id UUID,
        ip_address TEXT NOT NULL,
        port INT,
        xaddr TEXT,
        manufacturer TEXT,
        model TEXT,
        firmware TEXT,
        serial_number TEXT,
        hostname TEXT,
        capabilities JSONB DEFAULT '{}',
        onvif_data JSONB,
        is_new BOOLEAN DEFAULT TRUE,
        already_in_db BOOLEAN DEFAULT FALSE,
        imported BOOLEAN DEFAULT FALSE,
        imported_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW()
    )`)
	t.Cleanup(func() {
		db.MustExec("DROP TABLE IF EXISTS discovery_results")
		db.MustExec("DROP TABLE IF EXISTS discovery_scans")
		db.Close()
	})
	return NewResultStore(db, testLogger())
}

func TestCreateAndGetScan(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	id := uuid.New()
	siteID1 := uuid.New()
	scan := &ScanRecord{
		ID: id, SiteID: &siteID1, Status: "pending",
		Methods: []string{"ws-discovery"}, Ports: []int{80, 554},
	}
	if err := store.CreateScan(ctx, scan); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetScan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" {
		t.Errorf("expected pending, got %s", got.Status)
	}
}

func TestUpdateScanStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	id := uuid.New()
	siteID2 := uuid.New()
	store.CreateScan(ctx, &ScanRecord{ID: id, SiteID: &siteID2, Status: "pending", Methods: []string{"test"}})
	errMsg := "something went wrong"
	if err := store.UpdateScanStatus(ctx, id, "failed", 5, &errMsg); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetScan(ctx, id)
	if got.Status != "failed" {
		t.Errorf("expected failed, got %s", got.Status)
	}
	if got.TotalFound != 5 {
		t.Errorf("expected 5, got %d", got.TotalFound)
	}
	if got.Error == nil || *got.Error != errMsg {
		t.Errorf("expected '%s', got %v", errMsg, got.Error)
	}
}

func TestInsertAndGetResults(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	scanID := uuid.New()
	siteID3 := uuid.New()
	siteID4 := uuid.New()
	store.CreateScan(ctx, &ScanRecord{ID: scanID, SiteID: &siteID3, Status: "completed", Methods: []string{"test"}})
	resultID := uuid.New()
	xaddr := "http://10.0.0.1/onvif"
	result := &ResultRecord{
		ID: resultID, ScanID: scanID, SiteID: &siteID4,
		IPAddress: "10.0.0.1", XAddr: &xaddr,
		Capabilities: map[string]interface{}{"ptz": true, "media": true},
	}
	if err := store.InsertResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	results, total, err := store.GetResults(ctx, scanID, 1, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("expected 1 result, got %d", total)
	}
	if results[0].IPAddress != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", results[0].IPAddress)
	}
}

func TestGetScansPagination(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	siteID6 := uuid.New()
	for i := 0; i < 5; i++ {
		store.CreateScan(ctx, &ScanRecord{ID: uuid.New(), SiteID: &siteID6, Status: "completed", Methods: []string{"test"}})
	}
	scans, total, err := store.GetScans(ctx, &siteID6, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("expected 5 total, got %d", total)
	}
	if len(scans) != 3 {
		t.Errorf("expected 3 scans on page 1, got %d", len(scans))
	}
}

func TestMarkImported(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	scanID := uuid.New()
	siteID7 := uuid.New()
	siteID8 := uuid.New()
	siteID9 := uuid.New()
	store.CreateScan(ctx, &ScanRecord{ID: scanID, SiteID: &siteID7, Status: "completed", Methods: []string{"test"}})
	id1, id2 := uuid.New(), uuid.New()
	xaddr := "http://10.0.0.1/onvif"
	store.InsertResult(ctx, &ResultRecord{ID: id1, ScanID: scanID, SiteID: &siteID8, IPAddress: "10.0.0.1", XAddr: &xaddr})
	store.InsertResult(ctx, &ResultRecord{ID: id2, ScanID: scanID, SiteID: &siteID9, IPAddress: "10.0.0.2"})
	if err := store.MarkImported(ctx, []uuid.UUID{id1}); err != nil {
		t.Fatal(err)
	}
	results, _, _ := store.GetResults(ctx, scanID, 1, 20, "")
	for _, r := range results {
		if r.ID == id1 && !r.Imported {
			t.Error("expected id1 to be imported")
		}
		if r.ID == id2 && r.Imported {
			t.Error("expected id2 to NOT be imported")
		}
	}
}
