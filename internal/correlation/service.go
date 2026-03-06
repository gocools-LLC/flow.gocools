package correlation

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/graph"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/signals"
)

type Query struct {
	Start      time.Time
	End        time.Time
	MaxSkew    time.Duration
	ResourceID string
	LimitNodes int
	LimitEdges int
}

type Node struct {
	ID         string            `json:"id"`
	Kind       graph.NodeKind    `json:"kind"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type Edge struct {
	From string         `json:"from"`
	To   string         `json:"to"`
	Kind graph.EdgeKind `json:"kind"`
}

type Result struct {
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges"`
	MetricCount int    `json:"metric_count"`
	LogCount    int    `json:"log_count"`
}

type SignalStore interface {
	QueryMetricPoints(query signals.Query) []cloudwatch.MetricPoint
	QueryLogRecords(query signals.Query) []cloudwatchlogs.LogRecord
}

type Service struct {
	store SignalStore
}

func NewService(store SignalStore) *Service {
	return &Service{store: store}
}

func (s *Service) QueryGraph(query Query) (Result, error) {
	start := query.Start.UTC()
	end := query.End.UTC()
	if !start.IsZero() && !end.IsZero() && start.After(end) {
		return Result{}, errors.New("start must be before end")
	}
	if query.LimitNodes < 0 {
		return Result{}, errors.New("limit_nodes must be non-negative")
	}
	if query.LimitEdges < 0 {
		return Result{}, errors.New("limit_edges must be non-negative")
	}

	if s.store == nil {
		return Result{
			Nodes: []Node{},
			Edges: []Edge{},
		}, nil
	}

	metrics := s.store.QueryMetricPoints(signals.Query{
		Start: start,
		End:   end,
	})
	logs := s.store.QueryLogRecords(signals.Query{
		Start: start,
		End:   end,
	})

	correlationGraph := graph.BuildCorrelationGraph(metrics, logs, graph.CorrelationBuildConfig{
		MaxSkew: query.MaxSkew,
	})

	resourceID := strings.TrimSpace(query.ResourceID)
	nodes := correlationGraph.Nodes()
	if resourceID != "" {
		filteredNodes := make([]graph.Node, 0, len(nodes))
		for _, node := range nodes {
			if node.Attributes["resource_id"] != resourceID {
				continue
			}
			filteredNodes = append(filteredNodes, node)
		}
		nodes = filteredNodes
	}

	if query.LimitNodes > 0 && len(nodes) > query.LimitNodes {
		nodes = nodes[:query.LimitNodes]
	}

	allowedNodeIDs := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		allowedNodeIDs[node.ID] = struct{}{}
	}

	resultNodes := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		resultNodes = append(resultNodes, Node{
			ID:         node.ID,
			Kind:       node.Kind,
			Attributes: node.Attributes,
		})
	}

	edges := correlationGraph.Edges()
	resultEdges := make([]Edge, 0, len(edges))
	for _, edge := range edges {
		if _, ok := allowedNodeIDs[edge.From]; !ok {
			continue
		}
		if _, ok := allowedNodeIDs[edge.To]; !ok {
			continue
		}
		resultEdges = append(resultEdges, Edge{
			From: edge.From,
			To:   edge.To,
			Kind: edge.Kind,
		})
	}
	slices.SortStableFunc(resultEdges, func(a, b Edge) int {
		if a.From < b.From {
			return -1
		}
		if a.From > b.From {
			return 1
		}
		if a.To < b.To {
			return -1
		}
		if a.To > b.To {
			return 1
		}
		if a.Kind < b.Kind {
			return -1
		}
		if a.Kind > b.Kind {
			return 1
		}
		return 0
	})

	if query.LimitEdges > 0 && len(resultEdges) > query.LimitEdges {
		resultEdges = resultEdges[:query.LimitEdges]
	}

	return Result{
		Nodes:       resultNodes,
		Edges:       resultEdges,
		MetricCount: len(metrics),
		LogCount:    len(logs),
	}, nil
}
