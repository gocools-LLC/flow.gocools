# RFC-0001: GoCools Platform Architecture

- Status: Draft
- Organization: gocools-LLC
- Repository: flow.gocools
- Last Updated: 2026-03-05

## Overview

GoCools is a cloud-native infrastructure ecosystem made of three projects:

- nard.gocools: decentralized runtime and protocol layer
- arch.gocools: infrastructure visualization and control layer
- flow.gocools: telemetry and analysis layer

## Goals

- infrastructure visibility as a graph
- safe operations with explicit guardrails
- fast debugging via correlated telemetry
- productivity improvements for DevOps and platform teams

## Stack Metadata Requirements

All resources created by GoCools tooling must include:

- `gocools:stack-id`
- `gocools:environment`
- `gocools:owner`

## Security Baseline

- temporary credentials via AWS STS AssumeRole
- least privilege by default
- environment isolation and policy checks

## Notes for This Repository

This RFC applies platform-wide; flow.gocools owns its implementation details while staying compatible with shared contracts.
