//go:build e2e && openshift

package e2e

import (
	"os"
	"testing"
)

// TestMain for the OpenShift test suite. Route discovery tests call pkg/k8s
// directly. TestOpenShiftMetricsPresent connects to obs-mcp only when
// OBS_MCP_URL is set; it skips otherwise.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
