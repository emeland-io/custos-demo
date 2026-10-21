package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	TestAdminEmail = "admin@notebook.test"
	TestUserEmail  = "user@notebook.test"
	TestPassword   = "notebook-e2e-password"

	// testPasswordHash is bcrypt.GenerateFromPassword(TestPassword,
	// bcrypt.DefaultCost), precomputed. A fixed constant rather than
	// hashing at render time keeps RenderDexConfig deterministic:
	// bcrypt salts randomly, so hashing on every call would make the
	// rendered config (and so its checksum/config annotation) differ
	// on every `up` even when nothing meaningful changed, forcing Dex
	// to restart every run instead of only when its actual inputs do.
	testPasswordHash = "$2a$10$sORM/nDCrb5JcWOniTAhbe9L1CVVMRnOsoKMfjkQZmUE5oImJVVIi"
)

type DexConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

const dexConfigTemplate = `
issuer: %s
storage:
  type: memory
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
staticClients:
  - id: %s
    name: %s
    secret: %s
    redirectURIs:
      - %s
enablePasswordDB: true
staticPasswords:
  - email: %q
    hash: %q
    username: "admin"
    userID: "admin-test-user"
  - email: %q
    hash: %q
    username: "user"
    userID: "regular-test-user"
`

func RenderDexConfig(cfg DexConfig) (string, error) {
	return fmt.Sprintf(dexConfigTemplate,
		cfg.Issuer, cfg.ClientID, cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL,
		TestAdminEmail, testPasswordHash,
		TestUserEmail, testPasswordHash,
	), nil
}

const dexManifestsTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: dex-config
  namespace: %[1]s
data:
  config.yaml: |
%[2]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dex
  namespace: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dex
  template:
    metadata:
      labels:
        app: dex
      annotations:
        checksum/config: "%[3]s"
    spec:
      containers:
        - name: dex
          image: ghcr.io/dexidp/dex:v2.41.1
          args: ["dex", "serve", "/etc/dex/config.yaml"]
          ports:
            - containerPort: 5556
          volumeMounts:
            - name: config
              mountPath: /etc/dex
      volumes:
        - name: config
          configMap:
            name: dex-config
---
apiVersion: v1
kind: Service
metadata:
  name: dex
  namespace: %[1]s
spec:
  selector:
    app: dex
  ports:
    - port: 5556
      targetPort: 5556
`

func indentYAML(s, prefix string) string {
	out := prefix
	for _, c := range s {
		out += string(c)
		if c == '\n' {
			out += prefix
		}
	}
	return out
}

func RenderDexManifests(namespace, configYAML string) string {
	checksum := sha256.Sum256([]byte(configYAML))
	return fmt.Sprintf(dexManifestsTemplate, namespace, indentYAML(configYAML, "    "), hex.EncodeToString(checksum[:]))
}

func EnsureDex(ctx context.Context, cfg DexConfig) error {
	configYAML, err := RenderDexConfig(cfg)
	if err != nil {
		return fmt.Errorf("rendering Dex config: %w", err)
	}
	manifests := RenderDexManifests(TestNamespace, configYAML)
	if err := ApplyManifest(ctx, manifests); err != nil {
		return fmt.Errorf("applying Dex manifests: %w", err)
	}
	return RolloutWait(ctx, TestNamespace, "deployment/dex")
}
