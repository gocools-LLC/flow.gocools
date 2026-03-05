package graph

import (
	"fmt"
	"slices"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

type NodeKind string

const (
	NodeKindResource NodeKind = "resource"
	NodeKindMetric   NodeKind = "metric"
	NodeKindLog      NodeKind = "log"
)

type EdgeKind string

const (
	EdgeKindEmitsMetric    EdgeKind = "emits_metric"
	EdgeKindEmitsLog       EdgeKind = "emits_log"
	EdgeKindCorrelatedWith EdgeKind = "correlated_with"
)

type Node struct {
	ID         string
	Kind       NodeKind
	Attributes map[string]string
}

type Edge struct {
	From string
	To   string
	Kind EdgeKind
}

type Graph struct {
	nodes map[string]Node
	edges []Edge
}

func NewGraph() *Graph {
	return &Graph{
		nodes: map[string]Node{},
		edges: []Edge{},
	}
}

func (g *Graph) AddNode(node Node) {
	if node.ID == "" {
		return
	}
	if _, exists := g.nodes[node.ID]; exists {
		return
	}
	g.nodes[node.ID] = node
}

func (g *Graph) AddEdge(edge Edge) {
	if edge.From == "" || edge.To == "" || edge.Kind == "" {
		return
	}
	if _, exists := g.nodes[edge.From]; !exists {
		return
	}
	if _, exists := g.nodes[edge.To]; !exists {
		return
	}
	for _, existing := range g.edges {
		if existing == edge {
			return
		}
	}
	g.edges = append(g.edges, edge)
}

func (g *Graph) Node(id string) (Node, bool) {
	node, ok := g.nodes[id]
	return node, ok
}

func (g *Graph) Nodes() []Node {
	nodes := make([]Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(a, b Node) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return nodes
}

func (g *Graph) Edges() []Edge {
	edges := make([]Edge, len(g.edges))
	copy(edges, g.edges)
	return edges
}

func (g *Graph) EdgesForNode(nodeID string) []Edge {
	edges := make([]Edge, 0)
	for _, edge := range g.edges {
		if edge.From == nodeID || edge.To == nodeID {
			edges = append(edges, edge)
		}
	}
	return edges
}

func (g *Graph) Neighbors(nodeID string) []Node {
	seen := map[string]struct{}{}
	neighbors := make([]Node, 0)

	for _, edge := range g.edges {
		neighborID := ""
		switch {
		case edge.From == nodeID:
			neighborID = edge.To
		case edge.To == nodeID:
			neighborID = edge.From
		}
		if neighborID == "" {
			continue
		}
		if _, exists := seen[neighborID]; exists {
			continue
		}
		neighbor, ok := g.nodes[neighborID]
		if !ok {
			continue
		}
		seen[neighborID] = struct{}{}
		neighbors = append(neighbors, neighbor)
	}

	slices.SortFunc(neighbors, func(a, b Node) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	return neighbors
}

func (g *Graph) RelatedByEdgeKind(nodeID string, kind EdgeKind) []Node {
	related := make([]Node, 0)
	seen := map[string]struct{}{}

	for _, edge := range g.edges {
		if edge.From != nodeID || edge.Kind != kind {
			continue
		}
		node, ok := g.nodes[edge.To]
		if !ok {
			continue
		}
		if _, exists := seen[node.ID]; exists {
			continue
		}
		seen[node.ID] = struct{}{}
		related = append(related, node)
	}

	slices.SortFunc(related, func(a, b Node) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	return related
}

func (g *Graph) HasEdge(from string, to string, kind EdgeKind) bool {
	for _, edge := range g.edges {
		if edge.From == from && edge.To == to && edge.Kind == kind {
			return true
		}
	}
	return false
}

type CorrelationBuildConfig struct {
	MaxSkew time.Duration
}

func BuildCorrelationGraph(metrics []cloudwatch.MetricPoint, logs []cloudwatchlogs.LogRecord, cfg CorrelationBuildConfig) *Graph {
	maxSkew := cfg.MaxSkew
	if maxSkew <= 0 {
		maxSkew = 2 * time.Minute
	}

	graph := NewGraph()

	type metricRef struct {
		nodeID    string
		timestamp time.Time
	}
	type logRef struct {
		nodeID    string
		timestamp time.Time
	}

	metricsByResource := map[string][]metricRef{}
	logsByResource := map[string][]logRef{}

	for i, point := range metrics {
		resourceID := point.ResourceID
		if resourceID == "" {
			resourceID = "unknown-resource"
		}

		resourceNodeID := "resource:" + resourceID
		metricNodeID := fmt.Sprintf("metric:%s:%s:%d:%d", resourceID, point.MetricName, point.Timestamp.UTC().UnixMilli(), i)

		graph.AddNode(Node{
			ID:   resourceNodeID,
			Kind: NodeKindResource,
			Attributes: map[string]string{
				"resource_id": resourceID,
			},
		})
		graph.AddNode(Node{
			ID:   metricNodeID,
			Kind: NodeKindMetric,
			Attributes: map[string]string{
				"resource_id": resourceID,
				"namespace":   point.Namespace,
				"metric_name": point.MetricName,
				"timestamp":   point.Timestamp.UTC().Format(time.RFC3339Nano),
				"value":       fmt.Sprintf("%v", point.Value),
			},
		})
		graph.AddEdge(Edge{
			From: resourceNodeID,
			To:   metricNodeID,
			Kind: EdgeKindEmitsMetric,
		})

		metricsByResource[resourceID] = append(metricsByResource[resourceID], metricRef{
			nodeID:    metricNodeID,
			timestamp: point.Timestamp.UTC(),
		})
	}

	for i, entry := range logs {
		resourceID := inferLogResourceID(entry)
		resourceNodeID := "resource:" + resourceID
		logNodeID := logNodeIDForEntry(entry, i)

		graph.AddNode(Node{
			ID:   resourceNodeID,
			Kind: NodeKindResource,
			Attributes: map[string]string{
				"resource_id": resourceID,
			},
		})
		graph.AddNode(Node{
			ID:   logNodeID,
			Kind: NodeKindLog,
			Attributes: map[string]string{
				"resource_id":     resourceID,
				"log_group_name":  entry.LogGroupName,
				"log_stream_name": entry.LogStreamName,
				"event_id":        entry.EventID,
				"timestamp":       entry.Timestamp.UTC().Format(time.RFC3339Nano),
			},
		})
		graph.AddEdge(Edge{
			From: resourceNodeID,
			To:   logNodeID,
			Kind: EdgeKindEmitsLog,
		})

		logsByResource[resourceID] = append(logsByResource[resourceID], logRef{
			nodeID:    logNodeID,
			timestamp: entry.Timestamp.UTC(),
		})
	}

	for resourceID, metricRefs := range metricsByResource {
		logRefs := logsByResource[resourceID]
		for _, metricRef := range metricRefs {
			for _, logRef := range logRefs {
				if absDuration(metricRef.timestamp.Sub(logRef.timestamp)) > maxSkew {
					continue
				}
				graph.AddEdge(Edge{
					From: metricRef.nodeID,
					To:   logRef.nodeID,
					Kind: EdgeKindCorrelatedWith,
				})
			}
		}
	}

	return graph
}

func inferLogResourceID(record cloudwatchlogs.LogRecord) string {
	if record.Fields != nil {
		if resourceID, ok := record.Fields["resource_id"]; ok && resourceID != "" {
			return resourceID
		}
		if instanceID, ok := record.Fields["instance_id"]; ok && instanceID != "" {
			return instanceID
		}
	}

	if record.LogGroupName != "" || record.LogStreamName != "" {
		return record.LogGroupName + "/" + record.LogStreamName
	}

	return "unknown-resource"
}

func logNodeIDForEntry(entry cloudwatchlogs.LogRecord, index int) string {
	if entry.EventID != "" {
		return "log:" + entry.EventID
	}
	return fmt.Sprintf("log:%s:%d:%d", entry.LogStreamName, entry.Timestamp.UTC().UnixMilli(), index)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
