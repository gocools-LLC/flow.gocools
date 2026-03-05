# Flow Architecture

## Purpose

Live infrastructure telemetry and analysis platform for cloud and platform teams.

## High-Level Model

GoCools platform layers:

```text
nard.gocools
   -> arch.gocools
      -> flow.gocools
```

This repository focuses on **Flow** and integrates with the other layers through APIs and shared stack metadata.

## Core Capabilities

- live infrastructure telemetry\n- CloudWatch metrics and logs correlation\n- infrastructure flow visualization\n- incident debugging and health monitoring

## Guardrails

All managed cloud resources must include:

```text
gocools:stack-id
gocools:environment
gocools:owner
```

Destructive actions require stack validation and environment-aware protections.
