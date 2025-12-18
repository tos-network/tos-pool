# Multi-Upstream RPC Failover

TOS Pool supports connecting to multiple TOS nodes with automatic health checking and failover. This ensures high availability even when individual nodes become unavailable.

## Features

- **Multiple Upstream Nodes**: Configure multiple TOS nodes with different priorities
- **Automatic Health Checking**: Periodic health checks detect node failures
- **Automatic Failover**: Seamlessly switches to healthy nodes when the active node fails
- **Recovery Detection**: Automatically detects when failed nodes recover
- **Weight-Based Selection**: Higher weight nodes are preferred
- **Admin API Monitoring**: Monitor upstream status via REST API

## Configuration

### Basic Configuration (Single Node)

For backward compatibility, you can still use a single node:

```yaml
node:
  url: "http://127.0.0.1:8545"
  timeout: 10s
```

### Multi-Upstream Configuration

Configure multiple upstream nodes for high availability:

```yaml
node:
  # Primary URL (optional if upstreams are configured)
  url: "http://127.0.0.1:8545"
  timeout: 10s

  # Multiple upstream nodes
  upstreams:
    - name: "primary"
      url: "http://node1.example.com:8545"
      timeout: 10s
      weight: 10

    - name: "backup-local"
      url: "http://127.0.0.1:8545"
      timeout: 5s
      weight: 5

    - name: "backup-remote"
      url: "http://node2.example.com:8545"
      timeout: 15s
      weight: 1

  # Health check settings
  health_check_interval: 5s    # How often to check node health
  health_check_timeout: 3s     # Timeout for health check requests
  max_failures: 3              # Failures before marking unhealthy
  recovery_threshold: 2        # Successes before marking healthy again
```

### Configuration Options

#### Node Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `url` | string | `http://127.0.0.1:8545` | Primary node URL (for backward compatibility) |
| `timeout` | duration | `10s` | Default timeout for RPC requests |
| `upstreams` | array | `[]` | List of upstream node configurations |
| `health_check_interval` | duration | `5s` | Interval between health checks |
| `health_check_timeout` | duration | `3s` | Timeout for health check requests |
| `max_failures` | int | `3` | Consecutive failures before marking unhealthy |
| `recovery_threshold` | int | `2` | Consecutive successes before marking healthy |

#### Upstream Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | URL | Friendly name for logging and monitoring |
| `url` | string | required | Node RPC endpoint URL |
| `timeout` | duration | global timeout | Per-upstream request timeout |
| `weight` | int | `1` | Priority weight (higher = preferred) |

## How It Works

### Node Selection

1. Nodes are sorted by weight (highest first)
2. The highest-weight healthy node is selected as active
3. If multiple nodes have the same weight, the one with the highest block height is preferred

### Health Checking

The pool performs periodic health checks on all configured upstreams:

1. Sends `tos_getBlockByNumber("latest")` to each node
2. Records response time and current block height
3. Tracks consecutive successes and failures

### Failover Process

When the active node fails:

1. The failure is recorded
2. After `max_failures` consecutive failures, the node is marked unhealthy
3. The pool automatically selects the next best healthy node
4. All subsequent requests are routed to the new active node

### Recovery Process

When a failed node recovers:

1. Health checks continue on unhealthy nodes
2. After `recovery_threshold` consecutive successes, the node is marked healthy
3. If the recovered node has higher weight than the current active, it becomes active

## Monitoring

### Admin API Endpoint

Monitor upstream status via the admin API:

```
GET /admin/upstreams
Authorization: Bearer <admin_password>
```

Response:

```json
{
  "upstreams": [
    {
      "name": "primary",
      "url": "http://node1.example.com:8545",
      "healthy": true,
      "response_time_ms": 45.2,
      "height": 1234567,
      "weight": 10,
      "fail_count": 0,
      "success_count": 156
    },
    {
      "name": "backup-local",
      "url": "http://127.0.0.1:8545",
      "healthy": true,
      "response_time_ms": 12.8,
      "height": 1234567,
      "weight": 5,
      "fail_count": 0,
      "success_count": 156
    },
    {
      "name": "backup-remote",
      "url": "http://node2.example.com:8545",
      "healthy": false,
      "response_time_ms": 0,
      "height": 0,
      "weight": 1,
      "fail_count": 5,
      "success_count": 0
    }
  ],
  "total": 3,
  "healthy": 2,
  "active": "primary"
}
```

### Log Messages

The pool logs upstream events:

```
INFO  Starting upstream manager with 3 nodes
INFO    [0] primary (weight=10)
INFO    [1] backup-local (weight=5)
INFO    [2] backup-remote (weight=1)
WARN  Upstream backup-remote marked UNHEALTHY after 3 failures: connection refused
INFO  Switched to upstream primary (idx=0, weight=10, height=1234567)
INFO  Upstream backup-remote recovered and marked HEALTHY (height=1234568, response=45ms)
```

## Best Practices

### Geographic Distribution

Deploy nodes in different locations for resilience:

```yaml
upstreams:
  - name: "us-east"
    url: "http://us-east.nodes.example.com:8545"
    weight: 10

  - name: "us-west"
    url: "http://us-west.nodes.example.com:8545"
    weight: 8

  - name: "eu-west"
    url: "http://eu-west.nodes.example.com:8545"
    weight: 5
```

### Local + Remote Fallback

Use a local node as primary with remote backups:

```yaml
upstreams:
  - name: "local"
    url: "http://127.0.0.1:8545"
    timeout: 5s
    weight: 100

  - name: "remote-backup"
    url: "http://backup.nodes.example.com:8545"
    timeout: 15s
    weight: 1
```

### Timeout Tuning

Adjust timeouts based on network conditions:

- **Local nodes**: 3-5 seconds
- **Same region**: 5-10 seconds
- **Cross-region**: 10-20 seconds

### Health Check Tuning

For production environments:

```yaml
node:
  health_check_interval: 5s     # Check frequently
  health_check_timeout: 3s      # Fail fast on slow nodes
  max_failures: 3               # Quick failover (15 seconds)
  recovery_threshold: 5         # Ensure stability before recovery
```

For development/testing:

```yaml
node:
  health_check_interval: 10s    # Less frequent checks
  health_check_timeout: 5s      # More tolerant
  max_failures: 5               # More patient
  recovery_threshold: 2         # Quick recovery
```

## Troubleshooting

### All Nodes Unhealthy

If all nodes become unhealthy:

1. Check network connectivity to all nodes
2. Verify node URLs are correct
3. Ensure nodes are synced and accepting connections
4. Check firewall rules

The pool will continue trying to use the last active node and will recover automatically when nodes become available.

### Frequent Failovers

If you see frequent failovers:

1. Increase `max_failures` to tolerate occasional timeouts
2. Increase `health_check_timeout` for slow networks
3. Check node performance and stability
4. Consider using nodes with more consistent latency

### Node Not Recovering

If a node doesn't recover after coming back online:

1. Verify the node is fully synced
2. Check that health check requests succeed
3. Wait for `recovery_threshold` successful checks
4. Check logs for health check errors
