package store

import (
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGet(t *testing.T) {
	db := newTestDB(t)

	id, err := db.Create(NewEntry{
		Name: "MSP Sentinel", Summary: "did the thing", Source: "closed",
		AuditTier: "audit", AuditDate: "2026-09-07",
		Found: 56, Fixed: 56, Accepted: 0, CriticalOpen: 0,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(id) != idLength {
		t.Fatalf("id length = %d, want %d", len(id), idLength)
	}

	e, err := db.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if e.Status != StatusPending {
		t.Fatalf("status = %q, want pending", e.Status)
	}
	if e.Name != "MSP Sentinel" {
		t.Fatalf("name = %q", e.Name)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.GetByID("nosuchid00000"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestApproveAndHideAndLists(t *testing.T) {
	db := newTestDB(t)

	id, err := db.Create(NewEntry{Name: "A", Summary: "s", Source: "public", RepoURL: "https://example.com/a", AuditTier: "checkpoint", AuditDate: "2026-01-01"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pending, err := db.ListPending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("ListPending = %v, %v", pending, err)
	}

	if err := db.Approve(id); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	approved, err := db.ListApproved()
	if err != nil || len(approved) != 1 {
		t.Fatalf("ListApproved = %v, %v", approved, err)
	}
	if !approved[0].ApprovedAt.Valid {
		t.Fatalf("approved_at not set")
	}

	if err := db.Hide(id); err != nil {
		t.Fatalf("Hide: %v", err)
	}
	hidden, err := db.ListHidden()
	if err != nil || len(hidden) != 1 {
		t.Fatalf("ListHidden = %v, %v", hidden, err)
	}

	if err := db.Approve("doesnotexist"); err != ErrNotFound {
		t.Fatalf("Approve unknown = %v, want ErrNotFound", err)
	}
}

func TestCountPending(t *testing.T) {
	db := newTestDB(t)
	n, err := db.CountPending()
	if err != nil || n != 0 {
		t.Fatalf("CountPending = %d, %v", n, err)
	}
	if _, err := db.Create(NewEntry{Name: "A", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	n, err = db.CountPending()
	if err != nil || n != 1 {
		t.Fatalf("CountPending = %d, %v", n, err)
	}
}
