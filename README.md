# cf-tunnel-operator
![Build](https://github.com/rajeshkio/cf-tunnel-operator/actions/workflows/build.yaml/badge.svg)
![Chart Release](https://github.com/rajeshkio/cf-tunnel-operator/actions/workflows/chart-release.yaml/badge.svg)
![Go Version](https://img.shields.io/badge/go-1.25.3-blue)
![License](https://img.shields.io/badge/license-MIT-green)
[![Docker](https://img.shields.io/badge/docker-rk90229%2Fcf--tunnel--operator-blue?logo=docker)](https://hub.docker.com/r/rk90229/cf-tunnel-operator)
[![Helm](https://img.shields.io/badge/helm-chart-blue?logo=helm)](https://rajeshkio.github.io/cf-tunnel-operator)

A Kubernetes operator that automatically manages Cloudflare Tunnel routing rules based on HTTPRoute resources in your cluster. No more manual dashboard updates every time you deploy a new service.

## Articles

This operator was built as a learning exercise. The full series documents the decisions, mistakes, and concepts encountered while building it:

1. [How to Set Up Cloudflare Tunnel on Kubernetes — and How It Actually Works Inside](https://blogs.learningdevops.com/how-to-set-up-cloudflare-tunnel-on-kubernetes-and-how-it-actually-works-inside-91e11a35c84c)
2. [Building a Kubernetes Operator from Scratch — Automate Cloudflare Tunnel with HTTPRoute](https://blogs.learningdevops.com/building-a-kubernetes-operator-from-scratch-automate-cloudflare-tunnel-with-httproute-421d6642dc11)
3. [controller-gen — Building a Custom Kubernetes CRD Without Kubebuilder](https://blogs.learningdevops.com/controller-gen-building-a-custom-kubernetes-crd-without-kubebuilder-6c8f1e809150)
4. [Securing Every Service Using Cloudflare Zero Trust — Without Touching the Dashboard](https://blogs.learningdevops.com/securing-every-service-using-cloudflare-zero-trust-without-touching-the-dashboard-f80c6b28fff2)


## The Problem

Running services in a home lab or private Kubernetes cluster on Proxmox has a fundamental networking problem:

- No static public IP address
- ISPs use carrier-grade NAT — port forwarding does not work
- Dynamic DNS is fragile and exposes your home IP
- Every new service requires manually adding a hostname rule in the Cloudflare dashboard
- Delete a service and forget to clean up? Orphaned tunnel rules pile up

## The Solution

`cloudflared` runs as a Deployment inside your cluster and maintains an outbound-only connection to Cloudflare's edge network. Your home IP is never exposed. This operator watches your HTTPRoute resources and automatically keeps the tunnel routing rules in sync.

```
Browser
  → DNS resolves to Cloudflare edge IP (not your home IP)
  → Cloudflare edge finds the tunnel for this hostname
  → Request travels through the tunnel to cloudflared pod in your cluster
  → cloudflared forwards to your internal service
  → Response travels back the same path
```

## How It Works

```
HTTPRoute created / updated / deleted
              ↓
   Operator reconcile triggered
              ↓
   GET current config from Cloudflare API
              ↓
   Compare desired state vs actual state
              ↓
   No change?  → skip (no unnecessary API calls)
   Different?  → PUT updated config to Cloudflare API
              ↓
   cloudflared picks up new rules automatically
```

### Deletion Safety

The operator adds a finalizer to every HTTPRoute it manages:

```
cloudflare-tunnel.rajesh-kumar.in/cleanup
```

When you delete an HTTPRoute, Kubernetes holds the deletion until the operator removes the tunnel rule from Cloudflare first. This prevents orphaned rules pointing to dead services.

## Prerequisites

- Kubernetes cluster with Gateway API CRDs installed
- Cilium or another Gateway API-compatible implementation
- `cloudflared` running as a Deployment inside the cluster using a **remotely managed tunnel** (created via Cloudflare dashboard or API, running with `TUNNEL_TOKEN`)
- Cloudflare API token with the following permissions:

| Type    | Resource          | Permission |
| ------- | ----------------- | ---------- |
| Account | Cloudflare Tunnel | Edit       |
| Zone    | DNS               | Edit       |

## Project Structure

```
cf-tunnel-operator/
├── main.go                          # operator entrypoint, wires manager and reconciler
├── controllers/
│   └── httproute_reconciler.go      # reconcile loop — watches HTTPRoutes, syncs to Cloudflare
├── pkg/
│   └── cloudflare/
│       ├── client.go                # Cloudflare API client (GET/PUT tunnel config)
│       └── types.go                 # TunnelRule and TunnelConfig types
├── cmd/
│   └── test/
│       └── main.go                  # local tool to test the Cloudflare client
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go     # registers API group cf-tunnel-operator.rajesh-kumar.in/v1alpha1
│       ├── tunnelstatus_types.go    # TunnelStatus CRD type definition
│       └── zz_generated.deepcopy.go # auto-generated DeepCopy methods (do not edit)
├── deploy/
│   ├── namespace.yaml               # cf-tunnel-operator-system namespace
│   ├── serviceaccount.yaml          # ServiceAccount for the operator pod
│   ├── clusterrole.yaml             # RBAC — watch HTTPRoutes across all namespaces
│   ├── clusterrolebinding.yaml      # bind ClusterRole to ServiceAccount
│   ├── deployment.yaml              # operator Deployment
│   └── crd/
│       └── cf-tunnel-operator.rajesh-kumar.in_tunnelstatuses.yaml  # TunnelStatus CRD manifest
├── cf-tunnel-operator/              # Helm chart
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
└── Dockerfile                       # multi-arch build (amd64 + arm64)
```

## Installation

### Option 1: Helm (recommended)

#### 1. Add the Helm repo

```bash
helm repo add cf-tunnel-operator https://rajeshkio.github.io/cf-tunnel-operator
helm repo update
```

#### 2. Create the credentials secret

> **Never pass credentials as Helm values.** Create the secret manually before installing.

```bash
kubectl create namespace cf-tunnel-operator-system

kubectl create secret generic cf-tunnel-operator-credentials \
  -n cf-tunnel-operator-system \
  --from-literal=CF_ACCOUNT_ID=your-account-id \
  --from-literal=CF_TUNNEL_ID=your-tunnel-id \
  --from-literal=CF_DNS_ZONE_ID=your-dns-zone-id \
  --from-literal=CF_API_TOKEN=your-api-token
```

#### 3. Install the chart

```bash
helm install cf-tunnel-operator cf-tunnel-operator/cf-tunnel-operator
```

#### 4. Verify

```bash
kubectl get pods -n cf-tunnel-operator-system
kubectl logs -n cf-tunnel-operator-system deploy/cf-tunnel-operator -f
```

#### Configuration

| Value                 | Default                                | Description                           |
| --------------------- | -------------------------------------- | ------------------------------------- |
| `image.repository`    | `docker.io/rk90229/cf-tunnel-operator` | Operator image                        |
| `image.tag`           | `main-2a6a468`                         | Image tag                             |
| `namespace`           | `cf-tunnel-operator-system`            | Namespace to deploy into              |
| `credentialsSecret`   | `cf-tunnel-operator-credentials`       | Secret holding Cloudflare credentials |
| `serviceAccount.name` | `cf-tunnel-operator`                   | ServiceAccount name                   |
| `clusterrole.name`    | `cf-tunnel-operator`                   | ClusterRole name                      |

To override values:

```bash
helm install cf-tunnel-operator cf-tunnel-operator/cf-tunnel-operator \
  --set image.tag=main-abc1234 \
  --set credentialsSecret=my-custom-secret
```

To upgrade:

```bash
helm upgrade cf-tunnel-operator cf-tunnel-operator/cf-tunnel-operator
```

To uninstall:

```bash
helm uninstall cf-tunnel-operator
```

---

### Option 2: Raw manifests

#### 1. Create the credentials secret

> **Never commit this secret to git.** `deploy/secret.yaml` is in `.gitignore`.

```bash
kubectl create namespace cf-tunnel-operator-system

kubectl create secret generic cf-tunnel-operator-credentials \
  -n cf-tunnel-operator-system \
  --from-literal=CF_ACCOUNT_ID=your-account-id \
  --from-literal=CF_TUNNEL_ID=your-tunnel-id \
  --from-literal=CF_DNS_ZONE_ID=your-dns-zone-id \
  --from-literal=CF_API_TOKEN=your-api-token
```

#### 2. Apply the manifests

```bash
kubectl apply -f deploy/namespace.yaml
kubectl apply -f deploy/serviceaccount.yaml
kubectl apply -f deploy/clusterrole.yaml
kubectl apply -f deploy/clusterrolebinding.yaml
kubectl apply -f deploy/crd/
kubectl apply -f deploy/deployment.yaml
```

#### 3. Verify

```bash
kubectl get pods -n cf-tunnel-operator-system
kubectl logs -n cf-tunnel-operator-system deploy/cf-tunnel-operator -f
```

## Usage

The operator watches **all HTTPRoutes across all namespaces** automatically. No annotations needed.

Create an HTTPRoute as you normally would:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app
  namespace: default
spec:
  hostnames:
    - my-app.example.com
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: my-gateway
      namespace: my-namespace
  rules:
    - backendRefs:
        - name: my-app-service
          port: 8080
```

### Backend Scheme and TLS

By default the operator builds `http://` service URLs. For backends that speak HTTPS (self-signed certs, internal TLS), use these annotations:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app
  namespace: default
  annotations:
    cf-tunnel-operator/backend-scheme: "https"
    cf-tunnel-operator/no-tls-verify: "true"
spec: ...
```

| Annotation                          | Values          | Default | Description                                       |
| ----------------------------------- | --------------- | ------- | ------------------------------------------------- |
| `cf-tunnel-operator/backend-scheme` | `http`, `https` | `http`  | Scheme used to connect to the backend service     |
| `cf-tunnel-operator/no-tls-verify`  | `true`, `false` | `false` | Skip TLS certificate verification for the backend |

The operator automatically adds this rule to your Cloudflare Tunnel:

```
hostname: my-app.example.com
service:  http://my-app-service.default.svc.cluster.local:8080
```

### Zero Trust Access

Add two annotations to lock a hostname behind Cloudflare Access. When the operator sees these, it creates an Access Application and an email-based allow policy in your Cloudflare account. Only users whose email addresses are in the policy can reach the service — everyone else sees a Cloudflare login page.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app
  namespace: default
  annotations:
    cf-tunnel-operator/zero-trust: "true"
    cf-tunnel-operator/zero-trust-emails: "user@example.com,another@example.com"
spec:
  hostnames:
    - my-app.example.com
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: my-gateway
      namespace: my-namespace
  rules:
    - backendRefs:
        - name: my-app-service
          port: 8080
```

```
| Annotation                              |  Values         | Default | Description                                       |
| ----------------------------------------| --------------- | ------- | ------------------------------------------------- |
| `cf-tunnel-operator/zero-trust`         | `true`, `false  | `false` | Enable Cloudflare Access for this hostname        |
| `cf-tunnel-operator/zero-trust-emails`  | `true`, `false` | `false` | Emails allowed through the Access policy.         |
```

When `zero-trust: "true"` is set, the operator:

1. Creates a Cloudflare Access Application named after the hostname
2. Creates an allow policy named `cto-<hostname>` with the specified emails in the `include` rule
3. On every reconcile, updates the policy if the email list changes
4. When the HTTPRoute is deleted, removes the Access Application from Cloudflare

The Access Application and Policy IDs are stored in the `TunnelStatus` resource for visibility.

> Requires `Access: Apps and Policies Write` permission on your Cloudflare API token.

Delete the HTTPRoute and the tunnel rule along with application and policy is removed automatically.

## Observability: TunnelStatus

The operator creates a `TunnelStatus` custom resource for every HTTPRoute it manages. These live in the operator namespace and show the current sync state without needing to open the Cloudflare dashboard or read logs.

```bash
kubectl get tunnelstatuses -n cf-tunnel-operator-system
NAME                                          AGE
cattle-neuvector-system-neuvector-httproute   105m
cattle-system-rancher-httproute               105m
mlops-mlflow                                  105m
```

Inspect any one for the full picture:

```bash
kubectl get tunnelstatuses mlops-mlflow -n cf-tunnel-operator-system -o yaml
```

```yaml
apiVersion: cf-tunnel-operator.rajesh-kumar.in/v1alpha1
kind: TunnelStatus
metadata:
  name: mlops-mlflow
  namespace: cf-tunnel-operator-system
spec:
  httpRouteName: mlflow
  httpRouteNamespace: mlops
status:
  backendService: http://mlflow.mlops.svc.cluster.local:5000
  hostname: mlflow.rajesh-kumar.in
  lastSyncTime: "2026-06-03T15:15:16Z"
  message: ""
  notlsverify: false
  scheme: http
  syncStatus: Success
```

For a Zero Trust-enabled route:

```yaml
status:
  appid: aca224fa-43eb-4478-840b-cef99a751d7e
  backendService: http://my-app.default.svc.cluster.local:8080
  hostname: my-app.example.com
  lastSyncTime: "2026-06-16T07:13:38Z"
  message: ""
  notlsverify: false
  policyid: bcf0a040-feb2-43e7-99a7-ceea60530f53
  scheme: http
  syncStatus: Success
```

| Field            | Description                                          |
| ---------------- | ---------------------------------------------------- |
| `hostname`       | The public hostname registered in Cloudflare         |
| `backendService` | The internal service URL pushed to the tunnel        |
| `scheme`         | `http` or `https` depending on the annotation        |
| `notlsverify`    | Whether TLS verification is disabled for the backend |
| `syncStatus`     | `Success` or `Failed`                                |
| `message`        | Error detail if `syncStatus` is `Failed`             |
| `lastSyncTime`   | When the last reconcile ran                          |
| `appid`          | Cloudflare Access Application ID (Zero Trust only)   |
| `policyid`       | Cloudflare Access Policy ID (Zero Trust only)        |

If `syncStatus` is `Failed`, the `message` field contains the error from the Cloudflare API or DNS call. Fix the underlying issue and the operator retries automatically.

To apply the CRD to a cluster that does not have it yet:

```bash
kubectl apply -f deploy/crd/cf-tunnel-operator.rajesh-kumar.in_tunnelstatuses.yaml
```

## Local Development

Run the operator locally against your cluster using your current kubeconfig:

```bash
export CF_ACCOUNT_ID=your-account-id
export CF_TUNNEL_ID=your-tunnel-id
export CF_DNS_ZONE_ID=your-zone-id
export CF_API_TOKEN=your-api-token
export POD_NAMESPACE=cf-tunnel-operator-system

make run
```

`POD_NAMESPACE` is injected automatically via the Kubernetes Downward API when running in the cluster. For local runs you set it manually — it controls which namespace TunnelStatus resources are created in.

### Makefile Targets

| Target                   | Description                                                                          |
| ------------------------ | ------------------------------------------------------------------------------------ |
| `make build `            | Compile the operator binary                                                          |
| `make run`               | Run the operator locally using `KUBECONFIG` env var                                  |
| `make generate-crd`      | Run controller-gen and copy CRD to both `deploy/crd/` and `cf-tunnel-operator/crds/` |
| `make generate-deepcopy` | Regenerate DeepCopy methods after changing API types                                 |
| `make deploy-crd`        | Apply CRD to cluster using `KUBECONFIG` env var                                      |

To test the Cloudflare API client in isolation:

```bash
go run cmd/test/main.go
```

## Building

Multi-arch image for both `amd64` and `arm64`:

```bash
docker buildx create --use --name multiarch

docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t your-registry/cf-tunnel-operator:v0.1.1 \
  --push .
```

## Known Limitations

- Watches all HTTPRoutes — no opt-in annotation support yet
- Only uses the first hostname and first backend ref per HTTPRoute
- Single worker — sequential reconciliation (safe but slower under high load)

## Roadmap

- [x] Opt-in Zero Trust Access via annotations
- [ ] Support for multiple hostnames per HTTPRoute
- [ ] TLSRoute support
- [x] Automatic DNS CNAME record creation
- [x] Helm chart
- [x] TunnelStatus CRD for observability
- [x] Cloudflare rate limit handling with automatic backoff
