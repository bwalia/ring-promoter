# Prompt: Add Enterprise-Grade Prometheus Metrics to Ring Promoter

You are enhancing the existing Ring Promoter project.

Repository:
[https://github.com/bwalia/ring-promoter](https://github.com/bwalia/ring-promoter)

## Goal

Implement a production-grade Prometheus metrics endpoint so Ring Promoter becomes fully observable.

The implementation should follow Prometheus best practices and expose actionable metrics that allow operators to build Grafana dashboards, SLOs, alerts, and capacity planning.

Do **not** add simple "request counter" metrics only. Design a comprehensive observability layer suitable for enterprise production deployments.

---

# First

Analyse the existing codebase.

Understand:

* HTTP server
* Promotion engine
* Worker pools
* Queue processing
* GitHub/GitLab integrations
* Kubernetes deployment engine
* Database/storage
* Scheduler
* Background jobs
* Existing logging
* Configuration

Determine where instrumentation should naturally live.

Do not duplicate instrumentation.

---

# Use Official Prometheus Libraries

Use

* prometheus/client_golang
* promhttp

Expose

```
GET /metrics
```

using the standard Prometheus handler.

---

# Instrument Everything

## HTTP Metrics

Expose

* request count
* request duration
* requests in flight
* response size
* request size
* status codes
* endpoint labels
* method labels

Metrics

```
ringpromoter_http_requests_total

ringpromoter_http_request_duration_seconds

ringpromoter_http_requests_in_flight

ringpromoter_http_response_size_bytes
```

---

# Promotion Metrics

Track every promotion.

Metrics

```
ringpromoter_promotions_total

ringpromoter_promotions_success_total

ringpromoter_promotions_failed_total

ringpromoter_promotions_running

ringpromoter_promotion_duration_seconds
```

Labels

```
application

environment

target

strategy

status

pipeline

provider
```

---

# Rollback Metrics

```
ringpromoter_rollbacks_total

ringpromoter_rollbacks_success_total

ringpromoter_rollbacks_failed_total

ringpromoter_rollback_duration_seconds
```

---

# Deployment Metrics

Track

```
Deployments started

Deployments finished

Deployments failed

Canary deployments

Blue Green deployments

Helm deployments

Kustomize deployments

GitOps syncs
```

Metrics

```
ringpromoter_deployments_total

ringpromoter_deployment_duration_seconds
```

Labels

```
cluster

namespace

application

provider

strategy
```

---

# Queue Metrics

Expose

Queue depth

Worker count

Queue latency

Queue processing duration

Failures

Retries

Dead letters

Metrics

```
ringpromoter_queue_size

ringpromoter_queue_processing_seconds

ringpromoter_queue_retries_total

ringpromoter_queue_failures_total
```

---

# GitHub Metrics

Track

Workflow polling

API latency

Webhook processing

Failures

Rate limits

Metrics

```
ringpromoter_github_api_requests_total

ringpromoter_github_api_duration_seconds

ringpromoter_github_rate_limit_remaining

ringpromoter_webhooks_total
```

---

# GitLab Metrics

Same level of detail.

---

# Kubernetes Metrics

Track

Deployments

Rollouts

ReplicaSets

Pods

Ingress

Services

Namespaces

Metrics

```
ringpromoter_kubernetes_api_requests_total

ringpromoter_kubernetes_api_duration_seconds

ringpromoter_kubernetes_rollouts_total

ringpromoter_kubernetes_rollout_failures_total
```

---

# Worker Metrics

Expose

```
workers running

workers busy

jobs completed

jobs failed

job latency

retry count
```

---

# Database Metrics

If applicable

```
connection pool

query duration

transactions

errors

connection count

pool exhaustion
```

---

# Cache Metrics

If Redis or in-memory cache exists

```
hits

misses

evictions

memory usage
```

---

# Configuration Metrics

Expose

```
configured environments

clusters

applications

promotion policies

registered users

active sessions
```

---

# Business Metrics

These are the most important.

Track

Applications managed

Promotion requests

Approvals

Rejected promotions

Manual promotions

Automatic promotions

Promotion policies evaluated

Policy failures

Approvals waiting

Promotion SLA

Metrics

```
ringpromoter_applications_total

ringpromoter_environments_total

ringpromoter_approval_queue

ringpromoter_policy_failures_total

ringpromoter_manual_promotions_total

ringpromoter_auto_promotions_total
```

---

# Feature Usage Metrics

Track which features customers actually use.

Examples

```
Canary

Blue Green

Rollback

GitHub

GitLab

Slack

Teams

Telegram

ArgoCD

Flux

Helm

Kustomize
```

This helps product development.

---

# Export Build Information

Expose

```
Version

Git SHA

Build Date

Go Version

Platform

Architecture
```

Metric

```
ringpromoter_build_info
```

---

# Runtime Metrics

Include Go runtime metrics

```
goroutines

memory

GC

heap

threads

CPU

file descriptors
```

using the official collectors.

---

# Custom Collectors

Implement collectors for

Promotion Engine

Worker Pool

Scheduler

Queue

Approval Engine

Kubernetes Clients

Git Providers

---

# Grafana Dashboards

Automatically create dashboards.

```
grafana/

    dashboards/

        overview.json

        promotions.json

        deployments.json

        github.json

        kubernetes.json

        queues.json

        workers.json

        runtime.json

        business.json

        executive.json
```

---

# Dashboard 1

Executive Dashboard

Show

* Promotions today
* Success rate
* Failed promotions
* Active deployments
* Average deployment duration
* MTTR
* Rollbacks
* Success percentage

---

# Dashboard 2

Operations Dashboard

Show

Live deployments

Running jobs

Worker utilisation

Queue depth

Deployment latency

Errors

---

# Dashboard 3

Kubernetes Dashboard

Show

Deployments

Namespaces

Clusters

Pod failures

Rollouts

Rollbacks

---

# Dashboard 4

GitHub Dashboard

Show

Workflow latency

API failures

Webhook latency

Rate limits

---

# Dashboard 5

Business Dashboard

Show

Applications

Customers

Promotion frequency

Feature usage

Top applications

Top environments

Most promoted services

---

# Alerts

Create Prometheus alert rules.

Examples

```
PromotionFailureRateHigh

DeploymentDurationTooHigh

NoPromotionsInLastHour

QueueBacklogGrowing

WorkerPoolExhausted

GitHubRateLimitLow

KubernetesErrorsHigh

PromotionLatencyHigh

RollbacksIncreasing

ApprovalQueueTooLarge
```

Store under

```
monitoring/prometheus/rules/
```

---

# Helm Integration

Update the Helm chart.

Add

```
metrics:
    enabled: true
```

Service

```
port: 9090
```

ServiceMonitor

PodMonitor

PrometheusRule

Labels compatible with kube-prometheus-stack.

---

# Kubernetes

Create

```
Service

ServiceMonitor

PodMonitor

PrometheusRule

NetworkPolicy
```

---

# Documentation

Create

```
docs/monitoring.md
```

Include

* Every metric
* Labels
* Metric naming conventions
* Dashboard screenshots (placeholders)
* Example PromQL queries
* Recording rules
* Alert explanations
* Capacity planning guidance
* SLO examples
* Troubleshooting guide

---

# Example PromQL Queries

Include useful queries such as:

* Promotion success rate (%)
* Average promotion duration
* 95th percentile deployment latency
* Failed promotions by environment
* Rollbacks by application
* Queue backlog over time
* Worker utilisation
* Top promoted services
* Kubernetes API latency
* GitHub API latency
* Deployments per hour
* Promotion throughput
* MTTR estimation
* Approval bottlenecks

---

# OpenTelemetry

Design the metrics package so it can later support:

* OpenTelemetry Metrics
* OTLP export
* Jaeger tracing
* Tempo
* Loki correlation

without major refactoring.

---

# Code Quality

Create a reusable internal package:

```
internal/metrics/
```

with:

* Metric registration
* Middleware
* Custom collectors
* Label helpers
* Histogram bucket definitions
* Unit tests
* Benchmarks
* Mock collectors

Ensure metrics have low cardinality by avoiding labels with unbounded values (such as commit SHAs or user IDs), are thread-safe, and introduce minimal performance overhead.

---

## Final Goal

Ring Promoter should expose a rich `/metrics` endpoint that makes it straightforward to build production-ready Grafana dashboards, define meaningful SLOs, and monitor promotion pipelines, deployment health, Kubernetes activity, Git provider integrations, worker pools, and overall business usage. The implementation should feel comparable to mature CNCF projects such as Argo CD, Prometheus Operator, and Tekton, with clear documentation, Helm integration, alerting rules, and extensibility for future OpenTelemetry support.

