# Docker Monitoring Implementation

This document describes the monitoring implementation that addresses the "Docker monitoring problem" as referenced in the DataDog blog post.

## Overview

The monitoring system implements multi-level monitoring across three isolation levels:

1. **Process Level** - Individual process monitoring within containers
2. **Container Level** - Container-specific metrics and isolation monitoring  
3. **Host Level** - System-wide host metrics and resource monitoring

## Architecture

The monitoring addresses the gap between different isolation levels as described in the monitoring problem:

| Aspect | Process | Container | Host |
|--------|---------|-----------|------|
| Spec | Source | Dockerfile | Kickstart |
| On disk | .TEXT | /var/lib/docker | / |
| In memory | PID | Container ID | Hostname |
| In network | Socket | veth* | eth* |
| Runtime context | server core | host | data center |
| Isolation | moderate: memory space, etc. | private OS view: own PID space, file system, network interfaces | full: including own page caches and kernel |

## Usage

### Monitor Host Level

```bash
./basic-docker monitor host
```

Shows system-wide metrics including:
- Hostname and uptime
- Memory usage and availability  
- CPU count and load average
- Disk usage
- Network interfaces (eth*)
- All containers on the host

### Monitor Process Level

```bash
./basic-docker monitor process <PID>
```

Shows process-specific metrics including:
- Process ID, name, and status
- Memory usage (RSS and virtual)
- CPU time and percentage
- Thread count
- Open file descriptors
- Socket information

### Monitor Container Level

```bash
./basic-docker monitor container <container-id>
```

Shows container-specific metrics including:
- Container ID, name, and status
- Memory usage and limits
- Network statistics (veth interfaces)
- Process list within container
- Namespace information
- Docker storage path

### Monitor All Levels

```bash
./basic-docker monitor all
```

Aggregates metrics from all monitoring levels in a single JSON output.

### Gap Analysis

```bash
./basic-docker monitor gap
```

Analyzes monitoring gaps between isolation levels:
- Process to container correlation gaps
- Container to host visibility gaps  
- Cross-level monitoring challenges

### Correlation Analysis

```bash
./basic-docker monitor correlation <container-id>
```

Shows correlation between monitoring levels for a specific container, displaying the mapping table and detailed metrics.

## Implementation Details

### Monitors

- `ProcessMonitor` - Reads from `/proc/[pid]/` files to gather process metrics
- `ContainerMonitor` - Combines process monitoring with container metadata
- `HostMonitor` - Aggregates system-wide statistics from `/proc/` and `/sys/`

### Metrics Collection

- **Process metrics**: Read from `/proc/[pid]/stat`, `/proc/[pid]/status`, and `/proc/[pid]/fd/`
- **Container metrics**: Combine process metrics with container directory information
- **Host metrics**: Read from `/proc/meminfo`, `/proc/loadavg`, `/proc/uptime`, and filesystem stats

### Gap Analysis

The monitoring system identifies three categories of gaps:

1. **Process to Container**: PID mapping, namespace isolation visibility, resource limit enforcement
2. **Container to Host**: Network isolation vs visibility, filesystem overlay access, resource allocation
3. **Cross-Level**: Transaction tracing, performance correlation, security event correlation

## Testing

Run monitoring tests:

```bash
go test -v -run ".*Monitor.*"
```

Run benchmarks:

```bash
go test -bench=BenchmarkMonitoring
```

## References

- [The Docker Monitoring Problem](https://www.datadoghq.com/blog/the-docker-monitoring-problem/)
- Process isolation and namespace documentation
- Container runtime specifications