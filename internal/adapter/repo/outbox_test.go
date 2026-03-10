package repo

import (
	"context"
	"testing"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/event"
)

// makeTestEvent creates a minimal IdentityEvent for outbox tests.
func makeTestEvent(id, subject string) *event.IdentityEvent {
	return &event.IdentityEvent{
		EventID:    id,
		EventType:  subject,
		AccountID:  1,
		LurusID:    "lurus-test",
		OccurredAt: time.Now().UTC(),
	}
}

// TestOutboxRepo_Insert_Success verifies an event can be inserted and retrieved.
func TestOutboxRepo_Insert_Success(t *testing.T) {
	db := setupOutboxDB(t)
	r := NewOutboxRepo(db)
	ctx := context.Background()

	ev := makeTestEvent("evt-001", event.SubjectAccountCreated)
	if err := r.Insert(ctx, ev); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	rows, err := r.ListUnpublished(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].EventID != "evt-001" {
		t.Errorf("EventID = %s, want evt-001", rows[0].EventID)
	}
	if rows[0].Subject != event.SubjectAccountCreated {
		t.Errorf("Subject = %s, want %s", rows[0].Subject, event.SubjectAccountCreated)
	}
}

// TestOutboxRepo_ListUnpublished_ReturnsOnlyUnpublished verifies published events are excluded.
func TestOutboxRepo_ListUnpublished_ReturnsOnlyUnpublished(t *testing.T) {
	db := setupOutboxDB(t)
	r := NewOutboxRepo(db)
	ctx := context.Background()

	_ = r.Insert(ctx, makeTestEvent("evt-A", event.SubjectAccountCreated))
	_ = r.Insert(ctx, makeTestEvent("evt-B", event.SubjectTopupCompleted))

	// Get IDs, then mark one published.
	all, _ := r.ListUnpublished(ctx, 10)
	if len(all) != 2 {
		t.Fatalf("expected 2 unpublished, got %d", len(all))
	}
	if err := r.MarkPublished(ctx, all[0].ID); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	unpublished, err := r.ListUnpublished(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnpublished after mark: %v", err)
	}
	if len(unpublished) != 1 {
		t.Errorf("expected 1 unpublished, got %d", len(unpublished))
	}
}

// TestOutboxRepo_ListUnpublished_LimitRespected verifies the limit parameter is honoured.
func TestOutboxRepo_ListUnpublished_LimitRespected(t *testing.T) {
	db := setupOutboxDB(t)
	r := NewOutboxRepo(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = r.Insert(ctx, makeTestEvent("evt-lim-"+string(rune('0'+i)), event.SubjectAccountCreated))
	}

	rows, err := r.ListUnpublished(ctx, 3)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows with limit=3, got %d", len(rows))
	}
}

// TestOutboxRepo_MarkPublished_SetsTimestamp verifies published_at is set after marking.
func TestOutboxRepo_MarkPublished_SetsTimestamp(t *testing.T) {
	db := setupOutboxDB(t)
	r := NewOutboxRepo(db)
	ctx := context.Background()

	_ = r.Insert(ctx, makeTestEvent("evt-mark", event.SubjectAccountCreated))
	rows, _ := r.ListUnpublished(ctx, 10)
	if len(rows) == 0 {
		t.Fatal("expected inserted event")
	}
	id := rows[0].ID

	if err := r.MarkPublished(ctx, id); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	// Verify event no longer appears in unpublished list.
	remaining, _ := r.ListUnpublished(ctx, 10)
	for _, row := range remaining {
		if row.ID == id {
			t.Error("published event should not appear in unpublished list")
		}
	}
}

// TestOutboxRepo_IncrementAttempts_Success verifies attempts counter and last_error are updated.
func TestOutboxRepo_IncrementAttempts_Success(t *testing.T) {
	db := setupOutboxDB(t)
	r := NewOutboxRepo(db)
	ctx := context.Background()

	_ = r.Insert(ctx, makeTestEvent("evt-inc", event.SubjectAccountCreated))
	rows, _ := r.ListUnpublished(ctx, 10)
	if len(rows) == 0 {
		t.Fatal("expected inserted event")
	}
	id := rows[0].ID

	if err := r.IncrementAttempts(ctx, id, "connection refused"); err != nil {
		t.Fatalf("IncrementAttempts: %v", err)
	}

	// Verify by re-querying (attempts should be 1).
	updated, _ := r.ListUnpublished(ctx, 10)
	if len(updated) == 0 {
		t.Fatal("expected event still present")
	}
	if updated[0].Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", updated[0].Attempts)
	}
	if updated[0].LastError != "connection refused" {
		t.Errorf("LastError = %q, want %q", updated[0].LastError, "connection refused")
	}
}

// TestOutboxRepo_DeletePublishedBefore_Success verifies published events before cutoff are removed.
func TestOutboxRepo_DeletePublishedBefore_Success(t *testing.T) {
	db := setupOutboxDB(t)
	r := NewOutboxRepo(db)
	ctx := context.Background()

	// Insert two events and mark both published.
	_ = r.Insert(ctx, makeTestEvent("evt-del-1", event.SubjectAccountCreated))
	_ = r.Insert(ctx, makeTestEvent("evt-del-2", event.SubjectTopupCompleted))
	rows, _ := r.ListUnpublished(ctx, 10)
	for _, row := range rows {
		_ = r.MarkPublished(ctx, row.ID)
	}

	// Delete with future cutoff — should delete both published events.
	cutoff := time.Now().UTC().Add(time.Minute)
	deleted, err := r.DeletePublishedBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeletePublishedBefore: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
}

// TestOutboxRepo_DeletePublishedBefore_NothingToDelete verifies 0 is returned when nothing qualifies.
func TestOutboxRepo_DeletePublishedBefore_NothingToDelete(t *testing.T) {
	db := setupOutboxDB(t)
	r := NewOutboxRepo(db)
	ctx := context.Background()

	// Insert one event but do NOT mark published.
	_ = r.Insert(ctx, makeTestEvent("evt-keep", event.SubjectAccountCreated))

	// Past cutoff — nothing published yet.
	cutoff := time.Now().UTC().Add(-time.Hour)
	deleted, err := r.DeletePublishedBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("DeletePublishedBefore: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}
