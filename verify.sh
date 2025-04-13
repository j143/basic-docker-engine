#!/bin/bash
# Script to test Lean Docker Engine

# Create required directories
mkdir -p /tmp/basic-docker/containers
mkdir -p /tmp/basic-docker/images

# Build the basic-docker binary with error handling
echo "==== Building Project ===="
if ! go build -o basic-docker main.go; then
    echo "Error: Build failed. Please check the errors above." >&2
    exit 1
fi

# Display system information
echo "==== System Information ===="
./basic-docker info
echo "==========================="

# Run a simple command
echo -e "\n\n==== Running Simple Command ===="
sudo ./basic-docker run /bin/echo "Hello from container"

# List containers
echo -e "\n\n==== Listing Containers ===="
sudo ./basic-docker ps

# Run a more complex command if possible
if command -v busybox > /dev/null; then
    echo -e "\n\n==== Testing with busybox ===="
    sudo ./basic-docker run /bin/sh -c "ls -la /bin && echo 'Current directory:' && pwd"
fi

# Try to test isolation if we have privileges
if [ "$(id -u)" -eq 0 ]; then
    echo -e "\n\n==== Testing Isolation (requires root) ===="
    sudo ./basic-docker run /bin/sh -c "hostname container-test && hostname"
    
    # Test process isolation
    echo -e "\n==== Testing Process Isolation ===="
    sudo ./basic-docker run /bin/sh -c "echo 'PID list:' && ps aux"
else
    echo -e "\n\n==== Skipping isolation tests (needs root) ===="
fi


# Test the docker exec functionality
container_id=$(sudo ./basic-docker run /bin/sh -c "sleep 10" & echo $!)
sudo ./basic-docker ps
echo -e "\n\n==== Testing Docker Exec ===="
if sudo ./basic-docker exec $container_id ls /; then
    echo "[PASS] Docker exec command executed successfully."
else
    echo "[FAIL] Docker exec command failed."
fi

# Clean up the container
kill $container_id 2>/dev/null

echo -e "\n\n==== All tests completed ===="