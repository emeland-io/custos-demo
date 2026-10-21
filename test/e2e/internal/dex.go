package internal

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	TestAdminEmail = "admin@notebook.test"
	TestUserEmail  = "user@notebook.test"
	TestPassword   = "notebook-e2e-password"
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
	hash, err := bcrypt.GenerateFromPassword([]byte(TestPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing test password: %w", err)
	}
	return fmt.Sprintf(dexConfigTemplate,
		cfg.Issuer, cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL,
		TestAdminEmail, string(hash),
		TestUserEmail, string(hash),
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
	return fmt.Sprintf(dexManifestsTemplate, namespace, indentYAML(configYAML, "    "))
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
