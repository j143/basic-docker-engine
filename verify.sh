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
    
    # Test process isolation
    echo -e "\n==== Testing Process Isolation ===="
    sudo ./basic-docker run /bin/sh -c "echo 'PID list:' && ps aux"
else
    echo -e "\n\n==== Skipping isolation tests (needs root) ===="
fi

# Improved Docker Exec Test
container_id=$(sudo ./basic-docker run /bin/sh -c "sleep 30" & echo $!)
echo -e "\n\n==== Testing Docker Exec ===="

# Wait for the container to initialize
sleep 2

# Verify the container is running
if sudo ./basic-docker ps | grep -q "$container_id.*Running"; then
    echo "[PASS] Container is running."
else
    echo "[FAIL] Container is not running."
    kill $container_id 2>/dev/null
    exit 1
fi

# Execute a command inside the running container
if sudo ./basic-docker exec $container_id ls /; then
    echo "[PASS] Docker exec command executed successfully."
else
    echo "[FAIL] Docker exec command failed."
fi

# Clean up the container
kill $container_id 2>/dev/null
wait $container_id 2>/dev/null

# Verify the container is stopped
if sudo ./basic-docker ps | grep -q "$container_id.*Stopped"; then
    echo "[PASS] Container stopped successfully."
else
    echo "[FAIL] Container did not stop as expected."
fi

echo -e "\n\n==== All tests completed ===="