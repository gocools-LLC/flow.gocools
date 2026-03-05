# Telemetry Correlation Graph Model

This document defines how Flow models operational telemetry as a graph.

## Graph Entities

### Node kinds

- `resource`: infrastructure identity (for example `i-123` or `cluster-a/service-b`)
- `metric`: a point-in-time metric sample
- `log`: a normalized log event

### Edge kinds

- `emits_metric`: `resource -> metric`
- `emits_log`: `resource -> log`
- `correlated_with`: `metric -> log` when correlation rule matches

## Correlation Rule

Flow correlates a metric and log event when both conditions hold:

1. They resolve to the same resource ID.
2. Their timestamps are within `max_skew` (default: 2 minutes).

Resource resolution for logs prefers:

1. `fields.resource_id`
2. `fields.instance_id`
3. fallback `log_group_name/log_stream_name`

## In-Memory Query Support

The graph implementation exposes query helpers:

- `EdgesForNode(nodeID)` for incident traversal
- `Neighbors(nodeID)` for relationship expansion
- `RelatedByEdgeKind(nodeID, edgeKind)` for focused lookups

## Intended Usage

- incident debugging (metric spike -> related logs)
- architecture behavior analysis
- telemetry timeline enrichment

