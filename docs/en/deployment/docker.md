# Docker Deployment

## Overview

This guide covers deploying llm-proxy using Docker and Docker Compose. Docker provides an isolated, reproducible environment that simplifies deployment and scaling.

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/mutallipp/llm-proxy.git
cd llm-proxy
```

### 2. Configure Environment

Compose automatically loads `.env` from the project root:

```bash
cp .env.example .env
# Edit .env and set LLM_PROXY_DB_DSN and other local settings
```

Keep `.env` local; it is ignored by Git. The Compose file injects database, authentication, and proxy settings through `LLM_PROXY_*` variables.

### 3. Build and Start Services

```bash
docker compose up -d --build --force-recreate
```

Use `--build` on the first start and after code changes. The Dockerfile builds the frontend and embeds the latest Adapter and Model pages into the backend.

### 4. Verify Deployment

```bash
docker compose ps
curl http://localhost:8090/health
docker compose logs -f llm-proxy
```

Access the application at http://localhost:8090 and create the administrator account through the first-run setup wizard.

## Docker Compose Configuration

### Basic docker-compose.yml

```yaml
version: '3.8'

services:
  llm-proxy:
    image: llm-proxy:latest
    ports:
      - "8090:8090"
    volumes:
      - ./config.yml:/app/config.yml
      - llm-proxy_data:/app/data
    environment:
      - LLM_PROXY_SERVER_PORT=8090
      - LLM_PROXY_DB_DIALECT=sqlite3
      - LLM_PROXY_DB_DSN=file:llm-proxy.db?cache=shared&_fk=1
    restart: unless-stopped

volumes:
  llm-proxy_data:
```

### Production Configuration

```yaml
version: '3.8'

services:
  llm-proxy:
    image: llm-proxy:latest
    ports:
      - "8090:8090"
    volumes:
      - ./config.yml:/app/config.yml
      - llm-proxy_data:/app/data
      - ./logs:/app/logs
    environment:
      - LLM_PROXY_SERVER_PORT=8090
      - LLM_PROXY_DB_DIALECT=postgres
      - LLM_PROXY_DB_DSN=postgres://llm-proxy:password@postgres:5432/llm-proxy
      - LLM_PROXY_LOG_LEVEL=warn
      - LLM_PROXY_LOG_OUTPUT=file
      - LLM_PROXY_LOG_FILE_PATH=/app/logs/llm-proxy.log
    depends_on:
      - postgres
    restart: unless-stopped

  postgres:
    image: postgres:15
    environment:
      - POSTGRES_DB=llm-proxy
      - POSTGRES_USER=llm-proxy
      - POSTGRES_PASSWORD=password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    restart: unless-stopped

volumes:
  llm-proxy_data:
  postgres_data:
```

## Database Options

### SQLite (Development)

```yaml
llm-proxy:
  environment:
    - LLM_PROXY_DB_DIALECT=sqlite3
    - LLM_PROXY_DB_DSN=file:llm-proxy.db?cache=shared&_fk=1&_pragma=journal_mode(WAL)
```

### PostgreSQL (Production)

```yaml
llm-proxy:
  environment:
    - LLM_PROXY_DB_DIALECT=postgres
    - LLM_PROXY_DB_DSN=postgres://user:pass@host:5432/llm-proxy
```

### MySQL (Production)

```yaml
llm-proxy:
  environment:
    - LLM_PROXY_DB_DIALECT=mysql
    - LLM_PROXY_DB_DSN=user:pass@tcp(host:3306)/llm-proxy?charset=utf8mb4&parseTime=True
```

## Environment Variables

### Server Configuration

```bash
LLM_PROXY_SERVER_PORT=8090
LLM_PROXY_SERVER_NAME="llm-proxy"
LLM_PROXY_SERVER_DEBUG=false
LLM_PROXY_SERVER_REQUEST_TIMEOUT="30s"
LLM_PROXY_SERVER_LLM_REQUEST_TIMEOUT="600s"
```

### Database Configuration

```bash
LLM_PROXY_DB_DIALECT="postgres"
LLM_PROXY_DB_DSN="postgres://user:pass@host:5432/llm-proxy"
LLM_PROXY_DB_DEBUG=false
```

### Logging Configuration

```bash
LLM_PROXY_LOG_LEVEL="info"
LLM_PROXY_LOG_ENCODING="json"
LLM_PROXY_LOG_OUTPUT="stdio"
```

## Security Considerations

### Network Security

```yaml
llm-proxy:
  networks:
    - axonhub_network
  ports:
    - "127.0.0.1:8090:8090"  # Bind to localhost only

networks:
  axonhub_network:
    driver: bridge
```

### Secrets Management

Use Docker secrets or environment files:

```bash
# .env file
DB_PASSWORD=your-secure-password
API_KEY_SECRET=your-api-key-secret
```

```yaml
llm-proxy:
  env_file:
    - .env
```

## Monitoring and Logging

### Health Checks

```yaml
llm-proxy:
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:8090/health"]
    interval: 30s
    timeout: 10s
    retries: 3
    start_period: 40s
```

### Log Collection

```yaml
llm-proxy:
  logging:
    driver: "json-file"
    options:
      max-size: "10m"
      max-file: "3"
```

## Scaling

### Horizontal Scaling

```yaml
llm-proxy:
  deploy:
    replicas: 3
    resources:
      limits:
        memory: 1G
        cpus: '0.5'
      reservations:
        memory: 512M
        cpus: '0.25'
```

### Load Balancer Setup

```yaml
services:
  llm-proxy:
    image: llm-proxy:latest
    deploy:
      replicas: 3
    networks:
      - axonhub_network

  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    depends_on:
      - llm-proxy
    networks:
      - axonhub_network
```

## Backup and Recovery

### Database Backup

```yaml
services:
  backup:
    image: postgres:15
    volumes:
      - ./backup:/backup
      - postgres_data:/var/lib/postgresql/data
    command: |
      bash -c '
        pg_dump -h postgres -U llm-proxy llm-proxy > /backup/llm-proxy-$(date +%Y%m%d).sql
      '
    depends_on:
      - postgres
    environment:
      - PGPASSWORD=password
```

### Volume Backup

```bash
# Backup data volume
docker run --rm -v llm-proxy_data:/source -v $(pwd)/backup:/backup alpine \
  tar czf /backup/llm-proxy-data-$(date +%Y%m%d).tar.gz -C /source .

# Restore data volume
docker run --rm -v llm-proxy_data:/target -v $(pwd)/backup:/backup alpine \
  tar xzf /backup/llm-proxy-data-20231110.tar.gz -C /target
```

## Troubleshooting

### Common Issues

**Container fails to start**
- Check Docker logs: `docker-compose logs llm-proxy`
- Verify configuration file permissions
- Ensure database connection is working

**Port conflicts**
- Change the exposed port in docker-compose.yml
- Check if another service is using port 8090

**Database connection issues**
- Verify database credentials
- Check network connectivity between containers
- Ensure database container is running

### Debug Mode

Enable debug logging for troubleshooting:

```yaml
llm-proxy:
  environment:
    - LLM_PROXY_SERVER_DEBUG=true
    - LLM_PROXY_LOG_LEVEL=debug
```

## Next Steps

- [Configuration Guide](configuration.md)
- [OpenAI API](../api-reference/openai-api.md)
- [Anthropic API](../api-reference/anthropic-api.md)
- [Gemini API](../api-reference/gemini-api.md)
- [Architecture Documentation](../development/erd.md)
