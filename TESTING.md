# Testing

This document describes how to run tests for obs-mcp. Run `make help` to see all available targets.

## Linting

Run golangci-lint to check code quality:

```bash
make lint        # check
make lint-fix    # auto-fix
```

## Unit Tests

```bash
make test-unit
```

## Manual Testing

**OpenShift** — auto-discovers Thanos/Alertmanager routes via kubeconfig:

```bash
make run                       # kubeconfig auth, auto-discovers OpenShift Thanos route
make run-no-guardrails         # same, with guardrails disabled (use for Thanos < v0.40.0)
```

**kube-prometheus or any other backend** — set URLs explicitly (auto-discovery requires OpenShift Routes):

```bash
PROMETHEUS_URL=http://localhost:9090 ALERTMANAGER_URL=http://localhost:9093 make run
```

Override other defaults as needed:

```bash
LISTEN_ADDR=:8080 LOG_LEVEL=info make run
```

## Kind-based E2E Tests

Tests obs-mcp against a local Kind cluster with kube-prometheus.

```bash
make test-e2e-full          # setup + deploy + test + teardown in one command
```

Or step by step:

```bash
make test-e2e-setup         # create Kind cluster
make test-e2e-deploy        # build and deploy obs-mcp
make test-e2e               # run tests
make test-e2e-teardown      # cleanup
```

## OpenShift E2E Tests

Validates route auto-discovery (`pkg/k8s`) and tool correctness against OpenShift monitoring.

`TestRouteDiscovery_*` exercises `pkg/k8s` directly using the kubeconfig — no running obs-mcp needed.
`TestOpenShiftMetricsPresent` requires `OBS_MCP_URL` and is skipped when not set. In CI, `OBS_MCP_URL` is set automatically by the step registry to point at the deployed obs-mcp instance.

### Route discovery only

Verifies route auto-discovery, URL shape, and that each route responds HTTP 200 when accessed with the kubeconfig bearer token against a real `/api` endpoint.

```bash
make test-e2e-openshift
```

### Full suite including MCP tool smoke tests

Start obs-mcp in one terminal, then run the tests in another:

```bash
make run   # auto-discovers Thanos/Alertmanager routes via kubeconfig bearer token
```

```bash
OBS_MCP_URL=http://localhost:9100 make test-e2e-openshift   # OpenShift route discovery + metrics
OBS_MCP_URL=http://localhost:9100 make test-e2e             # full MCP tool smoke tests
```

> Note: `make test-e2e` without `OBS_MCP_URL` will attempt a port-forward to a Kind/k8s cluster. It will fail if no `obs-mcp` pod is running in the `obs-mcp` namespace.
