package timeline

import (
	"errors"
	"slices"
	"sync"
	"time"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

type Event struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Severity      Severity  `json:"severity"`
	Source        string    `json:"source"`
	Message       string    `json:"message"`
	CorrelationID string    `json:"correlation_id,omitempty"`
}

type Query struct {
	Start    time.Time
	End      time.Time
	Page     int
	PageSize int
}

type Page struct {
	Events   []Event `json:"events"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Total    int     `json:"total"`
}

type Service interface {
	QueryTimeline(query Query) (Page, error)
}

type InMemoryService struct {
	mu     sync.RWMutex
	events []Event
}

func NewInMemoryService(events []Event) *InMemoryService {
	copyEvents := make([]Event, len(events))
	copy(copyEvents, events)
	return &InMemoryService{
		events: copyEvents,
	}
}

func (s *InMemoryService) AddEvents(events ...Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, events...)
}

func (s *InMemoryService) QueryTimeline(query Query) (Page, error) {
	start := query.Start.UTC()
	end := query.End.UTC()
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return Page{}, errors.New("start must be before end")
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}

	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	s.mu.RLock()
	filtered := make([]Event, 0, len(s.events))
	for _, event := range s.events {
		ts := event.Timestamp.UTC()
		if !start.IsZero() && ts.Before(start) {
			continue
		}
		if !end.IsZero() && ts.After(end) {
			continue
		}
		filtered = append(filtered, event)
	}
	s.mu.RUnlock()

	slices.SortFunc(filtered, func(a, b Event) int {
		aTime := a.Timestamp.UTC()
		bTime := b.Timestamp.UTC()

		if aTime.After(bTime) {
			return -1
		}
		if aTime.Before(bTime) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	total := len(filtered)
	startOffset := (page - 1) * pageSize
	if startOffset >= total {
		return Page{
			Events:   []Event{},
			Page:     page,
			PageSize: pageSize,
			Total:    total,
		}, nil
	}

	endOffset := startOffset + pageSize
	if endOffset > total {
		endOffset = total
	}

	pageEvents := make([]Event, endOffset-startOffset)
	copy(pageEvents, filtered[startOffset:endOffset])

	return Page{
		Events:   pageEvents,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}
