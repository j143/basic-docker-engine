# basic-docker-engine

Basic docker engine implementation from scratch


## Build steps

### build go code

/workspaces/basic-docker-engine (main) $ go build -o basic-docker main.go 
/workspaces/basic-docker-engine (main) $ ./basic-docker 
Usage:
  basic-docker run <command> [args...]  - Run a command in a container
  basic-docker ps                      - List running containers
  basic-docker images                  - List available images

### create necessary folders

sudo mkdir -p /tmp/basic-docker/containers
sudo mkdir -p /tmp/basic-docker/images
sudo mkdir -p /sys/fs/cgroup/memory/basic-docker


### Set proper permissions

sudo chmod -R 755 /tmp/basic-docker
sudo chmod -R 755 /sys/fs/cgroup/memory/basic-docker


### Run a simple command in a container

> Note: This needs to be run as root due to namespace operations

sudo ./basic-docker run /bin/sh -c "echo Hello from container"


>  $ sudo ./basic-docker run /bin/sh -c "echo Hello from container"
> Starting container container-1743306338
> Error: failed to set memory limit: open /sys/fs/cgroup/memory/basic-docker/container-1743306338/memory.limit_in_bytes: permission denied
>

## Basic docker prompts

### `basic-docker run`

```bash
/workspaces/basic-docker-engine (main) $ sudo ./basic-docker run /bin/sh -c "echo Hello from container"
Starting container container-1743306338
Error: failed to set memory limit: open /sys/fs/cgroup/memory/basic-docker/container-1743306338/memory.limit_in_bytes: permission denied
```

### `basic-docker ps`

```bash
/workspaces/basic-docker-engine (main) $ sudo ./basic-docker ps
CONTAINER ID    STATUS  COMMAND
container-1743306338    N/A     N/A
```

### `basic-docker run /bin/sh`

```bash
/workspaces/basic-docker-engine (main) $ sudo ./basic-docker run /bin/sh -c "hostname isolated-container && hostname"
Starting container container-1743306481
Error: failed to set memory limit: open /sys/fs/cgroup/memory/basic-docker/container-1743306481/memory.limit_in_bytes: permission denied
```
