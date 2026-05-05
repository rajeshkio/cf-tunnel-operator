# cf-tunnel-operator

A Kubernetes operator that automatically manages Cloudflare Tunnel routing rules based on HTTPRoute resources in your cluster. No more manual dashboard updates every time you deploy a new service.

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
|---------|-------------------|------------|
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
├── deploy/
│   ├── namespace.yaml               # cf-tunnel-operator-system namespace
│   ├── serviceaccount.yaml          # ServiceAccount for the operator pod
│   ├── clusterrole.yaml             # RBAC — watch HTTPRoutes across all namespaces
│   ├── clusterrolebinding.yaml      # bind ClusterRole to ServiceAccount
│   └── deployment.yaml              # operator Deployment
└── Dockerfile                       # multi-arch build (amd64 + arm64)
```

## Installation

### 1. Create the credentials secret

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

### 2. Apply the manifests

```bash
kubectl apply -f deploy/namespace.yaml
kubectl apply -f deploy/serviceaccount.yaml
kubectl apply -f deploy/clusterrole.yaml
kubectl apply -f deploy/clusterrolebinding.yaml
kubectl apply -f deploy/deployment.yaml
```

### 3. Verify

```bash
# check the pod is running
kubectl get pods -n cf-tunnel-operator-system

# watch the logs
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

The operator automatically adds this rule to your Cloudflare Tunnel:

```
hostname: my-app.example.com
service:  http://my-app-service.default.svc.cluster.local:8080
```

Delete the HTTPRoute and the tunnel rule is removed automatically.

## Local Development

Run the operator locally against your cluster using your current kubeconfig:

```bash
export CF_ACCOUNT_ID=your-account-id
export CF_TUNNEL_ID=your-tunnel-id
export CF_DNS_ZONE_ID=your-zone-id
export CF_API_TOKEN=your-api-token

go run main.go
```

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
- No automatic DNS CNAME record creation yet
- Single worker — sequential reconciliation (safe but slower under high load)

## Roadmap

- [ ] Opt-in annotation (`cloudflare-tunnel/enabled: "true"`) for selective management
- [ ] Automatic DNS CNAME record creation
- [ ] Support for multiple hostnames per HTTPRoute
- [ ] TLSRoute support
- [ ] Helm chart
