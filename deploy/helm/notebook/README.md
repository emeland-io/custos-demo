# notebook Helm chart

Deploys the internal notebook app: a Go/Vue web app backed by
PostgreSQL via CloudNativePG.

## Cluster prerequisites (not installed by this chart)

This chart assumes the following are already installed on the target
cluster:

- **cert-manager**, with a `ClusterIssuer` matching whatever you pass
  as `ingress.tls.clusterIssuer`.
- **An ingress controller**, with an `IngressClass` matching whatever
  you pass as `ingress.className`.
- **The CloudNativePG operator**, so this chart's `Cluster` custom
  resource (`postgresql.cnpg.io/v1`) is reconciled into a running
  Postgres instance.
- **The Prometheus Operator**, so this chart's `PodMonitor` resource is
  picked up by your cluster's Prometheus instance.

This chart does not install any of the above and will fail at apply
time (for the CRDs it uses) if they're missing.

## Required secrets

This chart never creates Kubernetes `Secret` objects — create them
however your organization already manages secrets, then point the
chart at them via these values:

- `postgresql.credentialsSecretRef`: a secret with a key holding a full
  `postgres://` DSN, e.g. the connection string CloudNativePG generates
  for the `Cluster` this chart creates (CNPG auto-generates an
  `app`-user secret named `<cluster-name>-app`; for a release named
  `notebook`, the `Cluster` is named `notebook-notebook-postgres`, so
  the generated secret is `notebook-notebook-postgres-app`; point this
  value at that secret's `uri` key, or your own if you manage
  credentials differently).
- `oidc.clientIDSecretRef`, `oidc.clientSecretSecretRef`: your OIDC
  application's client ID and secret.
- `session.hashKeySecretRef`, `session.blockKeySecretRef`: two
  independently generated random values used to encrypt the app's
  session cookie. They have different length requirements, so do not
  generate them with the same command:
  - hash key: at least 32 characters, e.g.
    `openssl rand -hex 32` (produces 64 hex characters).
  - block key: exactly 16, 24, or 32 characters (selects
    AES-128/192/256), e.g. `openssl rand -hex 16` (produces 32 hex
    characters).

## Installing

```bash
helm install notebook deploy/helm/notebook \
  -f deploy/helm/notebook/values-prod.yaml \
  --set ingress.className=nginx \
  --set ingress.host=notebook.example.com \
  --set ingress.tls.clusterIssuer=letsencrypt-prod \
  --set postgresql.credentialsSecretRef.name=notebook-notebook-postgres-app \
  --set oidc.issuerURL=https://idp.example.com \
  --set oidc.redirectURL=https://notebook.example.com/auth/callback \
  --set oidc.clientIDSecretRef.name=notebook-oidc \
  --set oidc.clientSecretSecretRef.name=notebook-oidc \
  --set session.hashKeySecretRef.name=notebook-session \
  --set session.blockKeySecretRef.name=notebook-session
```
