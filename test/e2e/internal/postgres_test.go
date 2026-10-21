package internal_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"notebook/test/e2e/internal"
)

func TestRenderSelfSignedClusterIssuerManifest_IsValidYAML(t *testing.T) {
	manifest := internal.RenderSelfSignedClusterIssuerManifest()

	var parsed struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, manifest)
	}
	if parsed.Kind != "ClusterIssuer" {
		t.Fatalf("expected kind ClusterIssuer, got %s", parsed.Kind)
	}
}
