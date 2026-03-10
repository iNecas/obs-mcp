//go:build e2e && openshift

package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"k8s.io/client-go/rest"

	"github.com/rhobs/obs-mcp/pkg/k8s"
)

// authenticatedHTTPClient returns an HTTP client that sends the kubeconfig bearer
// token on every request and skips TLS verification for OpenShift ingress certs.
func authenticatedHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	restConfig, err := k8s.GetClientConfig()
	if err != nil {
		t.Fatalf("Failed to get kubeconfig: %v", err)
	}
	restConfig.TLSClientConfig = rest.TLSClientConfig{Insecure: true} //nolint:gosec
	rt, err := rest.TransportFor(restConfig)
	if err != nil {
		t.Fatalf("Failed to create authenticated transport: %v", err)
	}
	return &http.Client{Transport: rt}
}

// assertValidRouteURL checks that a discovered route URL is well-formed:
// - parseable by net/url
// - scheme is "https"
// - host is non-empty and contains a dot (i.e. not just a bare word)
func assertValidRouteURL(t *testing.T, raw string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Errorf("URL is not parseable: %s (%v)", raw, err)
		return
	}
	if parsed.Scheme != "https" {
		t.Errorf("Expected scheme 'https', got %q in URL: %s", parsed.Scheme, raw)
	}
	if parsed.Host == "" {
		t.Errorf("URL has no host: %s", raw)
	}
	if !strings.Contains(parsed.Host, ".") {
		t.Errorf("URL host looks invalid (no dot): %s", parsed.Host)
	}
}

// Route discovery tests below exercise pkg/k8s directly using the kubeconfig
// available to the test runner. They validate the auto-discovery path used when
// obs-mcp runs locally with --auth-mode kubeconfig. The deployed server in CI
// uses --auth-mode serviceaccount with URLs hardcoded in the configmap instead.

// TestRouteDiscovery_ThanosQuerier verifies that the thanos-querier route in
// openshift-monitoring can be discovered and returns a valid https:// URL.
func TestRouteDiscovery_ThanosQuerier(t *testing.T) {
	discoveredURL, err := k8s.GetMetricsBackendURL(k8s.MetricsBackendThanos)
	if err != nil {
		t.Fatalf("Failed to discover thanos-querier route: %v", err)
	}
	assertValidRouteURL(t, discoveredURL)
	t.Logf("Discovered Thanos URL: %s", discoveredURL)
}

// TestRouteDiscovery_PrometheusK8s verifies that the prometheus-k8s route in
// openshift-monitoring can be discovered when using the prometheus backend.
func TestRouteDiscovery_PrometheusK8s(t *testing.T) {
	discoveredURL, err := k8s.GetMetricsBackendURL(k8s.MetricsBackendPrometheus)
	if err != nil {
		t.Fatalf("Failed to discover prometheus-k8s route: %v", err)
	}
	assertValidRouteURL(t, discoveredURL)
	t.Logf("Discovered Prometheus URL: %s", discoveredURL)
}

// TestRouteDiscovery_Alertmanager verifies that the alertmanager-main route in
// openshift-monitoring can be discovered and returns a valid https:// URL.
func TestRouteDiscovery_Alertmanager(t *testing.T) {
	discoveredURL, err := k8s.GetAlertmanagerURL()
	if err != nil {
		t.Fatalf("Failed to discover alertmanager-main route: %v", err)
	}
	assertValidRouteURL(t, discoveredURL)
	t.Logf("Discovered Alertmanager URL: %s", discoveredURL)
}

// TestRouteDiscovery_URLsAreReachable verifies that the discovered route URLs respond
// with HTTP 200 when accessed with a valid bearer token against a real /api endpoint.
// Routes only expose /api paths — hitting the bare URL returns 503 regardless of auth.
func TestRouteDiscovery_URLsAreReachable(t *testing.T) {
	tests := []struct {
		name    string
		getURL  func() (string, error)
		apiPath string
	}{
		{
			name:    "thanos-querier",
			getURL:  func() (string, error) { return k8s.GetMetricsBackendURL(k8s.MetricsBackendThanos) },
			apiPath: "/api/v1/query?query=up",
		},
		{
			name:    "prometheus-k8s",
			getURL:  func() (string, error) { return k8s.GetMetricsBackendURL(k8s.MetricsBackendPrometheus) },
			apiPath: "/api/v1/query?query=up",
		},
		{
			name:    "alertmanager-main",
			getURL:  k8s.GetAlertmanagerURL,
			apiPath: "/api/v2/status",
		},
	}

	client := authenticatedHTTPClient(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawURL, err := tt.getURL()
			if err != nil {
				t.Fatalf("Route discovery failed: %v", err)
			}
			assertValidRouteURL(t, rawURL)

			apiURL := rawURL + tt.apiPath
			resp, err := client.Get(apiURL) //nolint:noctx
			if err != nil {
				t.Fatalf("Route %s (%s) is not reachable: %v", tt.name, apiURL, err)
			}
			defer resp.Body.Close()

			t.Logf("Route %s responded with HTTP %d", tt.name, resp.StatusCode)
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected HTTP 200 from %s, got %d", apiURL, resp.StatusCode)
			}
		})
	}
}

// TestOpenShiftMetricsPresent verifies that obs-mcp can query an OpenShift-specific
// metric, confirming it is wired to OpenShift in-cluster monitoring.
// Skipped when OBS_MCP_URL is not set.
func TestOpenShiftMetricsPresent(t *testing.T) {
	mcpURL := os.Getenv("OBS_MCP_URL")
	if mcpURL == "" {
		t.Skip("OBS_MCP_URL not set; skipping (set OBS_MCP_URL to run against a deployed or local obs-mcp)")
	}

	client := NewMCPClient(mcpURL)
	const metric = "cluster_monitoring_operator_reconcile_attempts_total"

	resp, err := client.CallTool(t, 1, "list_metrics", map[string]any{
		"name_regex": metric,
	})
	if err != nil {
		t.Fatalf("Failed to call list_metrics: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("MCP error: %s", resp.Error.Message)
	}
	if isErr, ok := resp.Result["isError"].(bool); ok && isErr {
		resultJSON, _ := json.Marshal(resp.Result)
		t.Fatalf("list_metrics returned error: %s", string(resultJSON))
	}

	resultJSON, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(resultJSON), metric) {
		t.Fatalf("OpenShift-specific metric %q not found -- is obs-mcp pointing at OpenShift monitoring?\nResult: %s", metric, string(resultJSON))
	}
	t.Logf("OpenShift metric %q confirmed present", metric)
}
