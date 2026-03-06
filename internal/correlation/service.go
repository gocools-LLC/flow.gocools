package correlation

import (
	"errors"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/graph"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/signals"
)

type Query struct {
	Start   time.Time
	End     time.Time
	MaxSkew time.Duration
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

	nodes := correlationGraph.Nodes()
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
		resultEdges = append(resultEdges, Edge{
			From: edge.From,
			To:   edge.To,
			Kind: edge.Kind,
		})
	}

	return Result{
		Nodes:       resultNodes,
		Edges:       resultEdges,
		MetricCount: len(metrics),
		LogCount:    len(logs),
	}, nil
}
