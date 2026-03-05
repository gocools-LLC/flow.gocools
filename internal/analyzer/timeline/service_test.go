package timeline

import (
	"testing"
	"time"
)

func TestQueryTimelineDeterministicOrderingWithTies(t *testing.T) {
	service := NewInMemoryService([]Event{
		{
			ID:        "event-b",
			Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:        "event-a",
			Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:        "event-c",
			Timestamp: time.Date(2026, 3, 5, 9, 59, 0, 0, time.UTC),
		},
	})

	page, err := service.QueryTimeline(Query{})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(page.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(page.Events))
	}

	if page.Events[0].ID != "event-a" || page.Events[1].ID != "event-b" || page.Events[2].ID != "event-c" {
		t.Fatalf("unexpected order: [%s %s %s]", page.Events[0].ID, page.Events[1].ID, page.Events[2].ID)
	}
}

func TestQueryTimelineTimeRangeAndPagination(t *testing.T) {
	service := NewInMemoryService([]Event{
		{ID: "event-1", Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)},
		{ID: "event-2", Timestamp: time.Date(2026, 3, 5, 10, 1, 0, 0, time.UTC)},
		{ID: "event-3", Timestamp: time.Date(2026, 3, 5, 10, 2, 0, 0, time.UTC)},
		{ID: "event-4", Timestamp: time.Date(2026, 3, 5, 10, 3, 0, 0, time.UTC)},
	})

	page, err := service.QueryTimeline(Query{
		Start:    time.Date(2026, 3, 5, 10, 1, 0, 0, time.UTC),
		End:      time.Date(2026, 3, 5, 10, 3, 0, 0, time.UTC),
		Page:     1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if page.Total != 3 {
		t.Fatalf("expected total 3, got %d", page.Total)
	}
	if len(page.Events) != 2 {
		t.Fatalf("expected 2 events on first page, got %d", len(page.Events))
	}
	if page.Events[0].ID != "event-4" || page.Events[1].ID != "event-3" {
		t.Fatalf("unexpected paged order: [%s %s]", page.Events[0].ID, page.Events[1].ID)
	}
}
