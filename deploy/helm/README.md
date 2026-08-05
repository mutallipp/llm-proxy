# llm-proxy Helm Chart

This Helm chart deploys llm-proxy on Kubernetes with PostgreSQL database.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PV provisioner support in the underlying infrastructure (for persistence)

## Installing the Chart

To install the chart with the release name `llm-proxy`:

```bash
helm install llm-proxy ./deploy/helm
```

## Configuration

The following table lists the configurable parameters of the llm-proxy chart and their default values.

### Global Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.imagePullSecrets` | Global Docker registry secret names as an array | `[]` |

### llm-proxy Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `llm-proxy.replicaCount` | Number of llm-proxy replicas | `1` |
| `llm-proxy.image.repository` | llm-proxy image repository | `llm-proxy` |
| `llm-proxy.image.tag` | llm-proxy image tag override. Defaults to chart `appVersion` when empty | `""` |
| `llm-proxy.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `llm-proxy.dbPassword` | Database password | `axonhub_password` |
| `llm-proxy.service.type` | Kubernetes service type | `ClusterIP` |
| `llm-proxy.service.port` | Service port | `8090` |
| `llm-proxy.resources` | CPU/Memory resource requests/limits | `{}` |
| `llm-proxy.persistence.enabled` | Enable persistence using PVC | `false` |
| `llm-proxy.persistence.size` | PVC storage request size | `10Gi` |

### PostgreSQL Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `postgresql.enabled` | Deploy PostgreSQL chart | `true` |
| `postgresql.replicaCount` | Number of PostgreSQL replicas | `1` |
| `postgresql.image.repository` | PostgreSQL image repository | `postgres` |
| `postgresql.image.tag` | PostgreSQL image tag | `16-alpine` |
| `postgresql.auth.postgresPassword` | PostgreSQL admin password | `axonhub_password` |
| `postgresql.auth.username` | PostgreSQL user name | `llm-proxy` |
| `postgresql.auth.password` | PostgreSQL user password | `axonhub_password` |
| `postgresql.auth.database` | PostgreSQL database name | `llm-proxy` |
| `postgresql.service.type` | Kubernetes service type | `ClusterIP` |
| `postgresql.service.port` | PostgreSQL service port | `5432` |
| `postgresql.primary.persistence.enabled` | Enable PostgreSQL persistence | `true` |
| `postgresql.primary.persistence.size` | PVC storage request size | `8Gi` |

### Ingress Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress controller resource | `false` |
| `ingress.className` | IngressClass that will be used | `""` |
| `ingress.annotations` | Ingress annotations | `{}` |
| `ingress.hosts` | Ingress hostnames | `[{host: llm-proxy.local, paths: [{path: /, pathType: Prefix}]}]` |
| `ingress.tls` | Ingress TLS configuration | `[]` |

## Database Configuration Options

### Using Internal PostgreSQL (Default)

The chart includes PostgreSQL by default. This is suitable for:
- Development environments
- Small production deployments
- When you want managed database within Kubernetes

```yaml
postgresql:
  enabled: true  # Default setting
```

### Using External Database

Disable internal PostgreSQL and configure external database connection:

```yaml
postgresql:
  enabled: false

llm-proxy:
  env:
    LLM_PROXY_DB_DSN: "postgres://username:password@external-db-host:5432/database?sslmode=require"
```

## Production Deployment

For production deployments, you should:

1. Change default passwords:
```yaml
llm-proxy:
  dbPassword: "your-secure-password"

postgresql:
  auth:
    postgresPassword: "your-secure-postgres-password"
    password: "your-secure-password"
```

2. Enable persistence:
```yaml
llm-proxy:
  persistence:
    enabled: true
    size: 20Gi

postgresql:
  primary:
    persistence:
      enabled: true
      size: 20Gi
```

3. Configure resource limits:
```yaml
llm-proxy:
  resources:
    limits:
      cpu: 2000m
      memory: 2Gi
    requests:
      cpu: 1000m
      memory: 1Gi

postgresql:
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 500m
      memory: 512Mi
```

4. Enable ingress for external access:
```yaml
ingress:
  enabled: true
  className: "nginx"
  hosts:
    - host: llm-proxy.yourdomain.com
      paths:
        - path: /
          pathType: Prefix
```

## Upgrading

To upgrade the chart:

```bash
helm upgrade llm-proxy ./deploy/helm -f values-production.yaml
```

## Uninstalling

To uninstall/delete the release:

```bash
helm uninstall llm-proxy
```

## Verification

After installation, you can verify the deployment:

```bash
# Check pods status
kubectl get pods

# Check services
kubectl get svc

# Port forward to test
kubectl port-forward svc/llm-proxy 8090:8090

# Test health endpoint
curl http://localhost:8090/health
```

## Troubleshooting

Common issues and solutions:

1. **Database connection failed**: Check PostgreSQL pod status and logs
2. **Insufficient resources**: Adjust resource requests/limits in values.yaml
3. **Persistent volume issues**: Ensure your cluster has PV provisioner configured
4. **Ingress not working**: Verify ingress controller is installed and configured

## Architecture

The chart deploys:
- llm-proxy application as a Deployment
- PostgreSQL database as a StatefulSet
- Services for both components
- Optional ingress for external access
- Persistent volumes for data persistence