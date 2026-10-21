package internal_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"notebook/test/e2e/internal"
)

// RenderDexManifests' checksum/config annotation only forces a
// rollout when the config actually changed (see
// TestRenderDexManifests_DeploymentChecksumAnnotationChangesWithConfig
// above) — but bcrypt.GenerateFromPassword salts randomly, so if
// RenderDexConfig re-hashed TestPassword on every call, the rendered
// config (and so the checksum) would differ on every `up`, even when
// nothing meaningful changed, forcing Dex to restart every run
// instead of only when its actual inputs change.
func TestRenderDexConfig_IsDeterministicAcrossCalls(t *testing.T) {
	cfg := internal.DexConfig{
		Issuer:       "http://dex.notebook-e2e.svc.cluster.local:5556",
		ClientID:     "notebook",
		ClientSecret: "notebook-secret",
		RedirectURL:  "http://notebook.notebook-e2e.svc.cluster.local/auth/callback",
	}

	first, err := internal.RenderDexConfig(cfg)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	second, err := internal.RenderDexConfig(cfg)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if first != second {
		t.Fatalf("expected RenderDexConfig to be deterministic for the same input, got two different renders")
	}
}

func TestRenderDexConfig_ProducesValidYAMLWithWorkingPasswordHash(t *testing.T) {
	cfg := internal.DexConfig{
		Issuer:       "http://dex.notebook-e2e.svc.cluster.local:5556",
		ClientID:     "notebook",
		ClientSecret: "notebook-secret",
		RedirectURL:  "http://notebook.notebook-e2e.svc.cluster.local/auth/callback",
	}

	rendered, err := internal.RenderDexConfig(cfg)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	var parsed struct {
		Issuer        string `yaml:"issuer"`
		StaticClients []struct {
			ID           string   `yaml:"id"`
			Name         string   `yaml:"name"`
			RedirectURIs []string `yaml:"redirectURIs"`
		} `yaml:"staticClients"`
		StaticPasswords []struct {
			Email string `yaml:"email"`
			Hash  string `yaml:"hash"`
		} `yaml:"staticPasswords"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, rendered)
	}

	if parsed.Issuer != cfg.Issuer {
		t.Fatalf("expected issuer %s, got %s", cfg.Issuer, parsed.Issuer)
	}
	if len(parsed.StaticClients) != 1 || parsed.StaticClients[0].ID != cfg.ClientID {
		t.Fatalf("expected one static client with id %s, got %+v", cfg.ClientID, parsed.StaticClients)
	}
	if parsed.StaticClients[0].Name == "" {
		t.Fatalf("expected the static client to have a non-empty name (Dex rejects staticClients without one): %+v", parsed.StaticClients[0])
	}
	if len(parsed.StaticPasswords) != 2 {
		t.Fatalf("expected 2 static passwords, got %d", len(parsed.StaticPasswords))
	}
	if err := bcrypt.CompareHashAndPassword([]byte(parsed.StaticPasswords[0].Hash), []byte(internal.TestPassword)); err != nil {
		t.Fatalf("expected rendered hash to verify against TestPassword: %v", err)
	}
}

func TestRenderDexManifests_ProducesValidMultiDocYAML(t *testing.T) {
	manifests := internal.RenderDexManifests("my-ns", "issuer: http://x\n")
	docs := splitYAMLDocs(manifests)
	if len(docs) < 3 {
		t.Fatalf("expected at least 3 documents (ConfigMap, Deployment, Service), got %d", len(docs))
	}
	for _, doc := range docs {
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Fatalf("invalid yaml document: %v\n%s", err, doc)
		}
	}
}

// Changing the ConfigMap's content alone never restarts a Deployment's
// existing pods (kubectl apply on an unchanged Deployment spec is a
// no-op), so a stale Dex config is otherwise silently never picked up
// by `devcluster up` re-runs. The Deployment's pod template must carry
// a checksum of the config so a content change forces a new rollout.
func TestRenderDexManifests_DeploymentChecksumAnnotationChangesWithConfig(t *testing.T) {
	manifestA := internal.RenderDexManifests("my-ns", "issuer: http://a\n")
	manifestB := internal.RenderDexManifests("my-ns", "issuer: http://b\n")

	checksumA := dexDeploymentChecksum(t, manifestA)
	checksumB := dexDeploymentChecksum(t, manifestB)

	if checksumA == "" {
		t.Fatalf("expected a non-empty checksum/config annotation on the Deployment, got manifest:\n%s", manifestA)
	}
	if checksumA == checksumB {
		t.Fatalf("expected the checksum/config annotation to differ when the rendered config differs, got %q for both", checksumA)
	}
}

func dexDeploymentChecksum(t *testing.T, manifests string) string {
	t.Helper()
	for _, doc := range splitYAMLDocs(manifests) {
		var parsed struct {
			Kind     string `yaml:"kind"`
			Spec     struct {
				Template struct {
					Metadata struct {
						Annotations map[string]string `yaml:"annotations"`
					} `yaml:"metadata"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Fatalf("invalid yaml document: %v\n%s", err, doc)
		}
		if parsed.Kind == "Deployment" {
			return parsed.Spec.Template.Metadata.Annotations["checksum/config"]
		}
	}
	t.Fatalf("no Deployment document found in manifests:\n%s", manifests)
	return ""
}

func splitYAMLDocs(s string) []string {
	var docs []string
	for _, part := range splitOn(s, "\n---\n") {
		trimmed := trimSpace(part)
		if trimmed != "" {
			docs = append(docs, trimmed)
		}
	}
	return docs
}

func splitOn(s, sep string) []string {
	var parts []string
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			parts = append(parts, s)
			return parts
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
	}
}

func indexOf(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
