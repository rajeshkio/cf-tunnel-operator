# cf-tunnel-operator Helm Chart

Deploys the cf-tunnel-operator — a Kubernetes operator that watches HTTPRoute resources and automatically syncs hostname routing rules to a Cloudflare Tunnel.

## Prerequisites

- Kubernetes cluster with Gateway API CRDs installed
- `cloudflared` running as a Deployment inside the cluster using a remotely managed tunnel (running with `TUNNEL_TOKEN`)
- Cloudflare API token with the following permissions:

| Type    | Resource          | Permission |
| ------- | ----------------- | ---------- |
| Account | Cloudflare Tunnel | Edit       |
| Zone    | DNS               | Edit       |

## Add the repo

```bash
helm repo add cf-tunnel-operator https://rajeshkio.github.io/cf-tunnel-operator
helm repo update
```

## Install

### 1. Create the credentials secret

> **Never pass credentials as Helm values.** The chart does not create the secret — you create it manually so credentials never touch Helm release history.

```bash
kubectl create namespace cf-tunnel-operator-system

kubectl create secret generic cf-tunnel-operator-credentials \
  -n cf-tunnel-operator-system \
  --from-literal=CF_ACCOUNT_ID=your-account-id \
  --from-literal=CF_TUNNEL_ID=your-tunnel-id \
  --from-literal=CF_DNS_ZONE_ID=your-dns-zone-id \
  --from-literal=CF_API_TOKEN=your-api-token
```

### 2. Install the chart

```bash
helm install cf-tunnel-operator cf-tunnel-operator/cf-tunnel-operator
```

### 3. Verify

```bash
kubectl get pods -n cf-tunnel-operator-system
kubectl logs -n cf-tunnel-operator-system deploy/cf-tunnel-operator -f
```

## Values

| Value                 | Default                                | Description                                       |
| --------------------- | -------------------------------------- | ------------------------------------------------- |
| `image.repository`    | `docker.io/rk90229/cf-tunnel-operator` | Operator image                                    |
| `image.tag`           | `main-2a6a468`                         | Image tag                                         |
| `namespace`           | `cf-tunnel-operator-system`            | Namespace to deploy into                          |
| `credentialsSecret`   | `cf-tunnel-operator-credentials`       | Name of the secret holding Cloudflare credentials |
| `serviceAccount.name` | `cf-tunnel-operator`                   | ServiceAccount name                               |
| `clusterrole.name`    | `cf-tunnel-operator`                   | ClusterRole name                                  |

To override values:

```bash
helm install cf-tunnel-operator cf-tunnel-operator/cf-tunnel-operator \
  --set image.tag=main-abc1234 \
  --set credentialsSecret=my-custom-secret
```

## What the chart deploys

- Namespace (`cf-tunnel-operator-system`)
- ServiceAccount
- ClusterRole with HTTPRoute watch permissions across all namespaces
- ClusterRoleBinding
- Deployment running the operator

## Upgrade

```bash
helm upgrade cf-tunnel-operator cf-tunnel-operator/cf-tunnel-operator
```

## Uninstall

```bash
helm uninstall cf-tunnel-operator
```
