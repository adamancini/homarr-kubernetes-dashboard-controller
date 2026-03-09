# Homarr Kubernetes Dashboard Controller

A Kubernetes controller that watches annotated resources and automatically syncs them to a [Homarr](https://homarr.dev) dashboard. Annotate your Ingresses, IngressRoutes, Services, or HTTPRoutes and the controller creates corresponding apps and integrations on a Homarr board via its tRPC API.

## Supported Sources

| Source | Flag | Watches |
|--------|------|---------|
| Kubernetes Ingress | `ingress` | `networking.k8s.io/v1` Ingress |
| Traefik IngressRoute | `traefik-proxy` | `traefik.io/v1alpha1` IngressRoute |
| Gateway API HTTPRoute | `gateway-httproute` | `gateway.networking.k8s.io/v1` HTTPRoute |
| Service | `service` | `v1` Service |

## How It Works

1. The controller polls annotated Kubernetes resources on a configurable interval (default 5m).
2. Resources with `homarr.dev/enabled: "true"` are collected as desired dashboard entries.
3. The controller reconciles desired state against Homarr — creating, updating, or deleting apps and integrations as needed.
4. Apps are placed onto a named Homarr board with optional integration linkage.

## Annotations

All annotations use a configurable prefix (default `homarr.dev`).

| Annotation | Required | Description |
|------------|----------|-------------|
| `homarr.dev/enabled` | yes | Set to `"true"` to manage this resource |
| `homarr.dev/name` | yes | Display name on the dashboard |
| `homarr.dev/url` | yes | App URL |
| `homarr.dev/icon` | no | Icon URL or name (resolved against `--default-icon-base-url`) |
| `homarr.dev/description` | no | App description |
| `homarr.dev/ping-url` | no | Health check URL |
| `homarr.dev/group` | no | Group name for organization |
| `homarr.dev/priority` | no | Sort priority (integer) |
| `homarr.dev/integration-type` | no | Homarr integration kind (e.g. `sonarr`, `radarr`) |
| `homarr.dev/integration-url` | no | Integration API URL (defaults to app URL) |
| `homarr.dev/integration-secret` | no | Kubernetes Secret name containing integration credentials |
| `homarr.dev/integration-secret-key` | no | Specific key within the Secret (reads as `apiKey`) |
| `homarr.dev/widget` | no | Widget type |

### Example

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: sonarr
  annotations:
    homarr.dev/enabled: "true"
    homarr.dev/name: "Sonarr"
    homarr.dev/url: "https://sonarr.example.com"
    homarr.dev/icon: "sonarr"
    homarr.dev/description: "TV series management"
    homarr.dev/integration-type: "sonarr"
    homarr.dev/integration-secret: "sonarr-api-key"
    homarr.dev/integration-secret-key: "api-key"
spec:
  rules:
    - host: sonarr.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: sonarr
                port:
                  number: 8989
```

## Installation

### Helm (recommended)

```bash
helm install homarr-dashboard-controller \
  oci://ghcr.io/adamancini/charts/homarr-kubernetes-dashboard-controller \
  --namespace homarr \
  --set homarr.url=http://homarr.homarr.svc:7575 \
  --set sources={ingress}
```

### Prerequisites

- Homarr v1 instance with API key authentication enabled
- A Kubernetes Secret containing the Homarr API key:

```bash
kubectl create secret generic homarr-api-key \
  --namespace homarr \
  --from-literal=api-key=YOUR_API_KEY
```

### Helm Values

See [`charts/homarr-kubernetes-dashboard-controller/values.yaml`](charts/homarr-kubernetes-dashboard-controller/values.yaml) for the full reference. Key values:

```yaml
homarr:
  url: "http://homarr.homarr.svc:7575"
  apiKeySecret:
    name: homarr-api-key
    key: api-key

sources:
  - ingress
  # - traefik-proxy
  # - gateway-httproute
  # - service

board:
  name: default
  columnCount: 12

annotationPrefix: "homarr.dev"
reconcileInterval: 5m
leaderElect: true

namespaces:
  allNamespaces: false
  watch: []
  ignore:
    - kube-system
    - flux-system
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | (required) | Source type to watch (repeatable) |
| `--homarr-url` | `http://homarr.homarr.svc:7575` | Homarr API base URL |
| `--board-name` | `default` | Board to manage |
| `--board-column-count` | `12` | Column count for auto-created boards |
| `--annotation-prefix` | `homarr.dev` | Annotation prefix |
| `--default-icon-base-url` | `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/svg` | Base URL for icon name resolution |
| `--namespace` | (none) | Namespace to watch (repeatable) |
| `--ignore-namespace` | `kube-system,flux-system` | Namespace to ignore (repeatable) |
| `--all-namespaces` | `false` | Watch all namespaces |
| `--reconcile-interval` | `5m` | Full resync interval |
| `--leader-elect` | `true` | Enable leader election |

The `HOMARR_API_KEY` environment variable is required at runtime.

## Development

```bash
# Build
make build

# Run tests
make test

# Lint
make lint

# Build Docker image
make docker-build VERSION=dev
```

### Project Structure

```
cmd/controller/       Entry point
internal/
  config/             Configuration and validation
  controller/         Reconciliation loop
  homarr/             Homarr tRPC API client
  source/             Kubernetes resource adapters
  state/              In-memory state tracking
charts/               Helm chart
```

## License

[MIT](LICENSE)
