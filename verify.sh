#!/bin/bash
# Script to test Lean Docker Engine

# Use a temporary directory for testing
BASE_DIR=$(mktemp -d)
CONTAINERS_DIR="$BASE_DIR/containers"
IMAGES_DIR="$BASE_DIR/images"

# Create required directories
mkdir -p "$CONTAINERS_DIR"
mkdir -p "$IMAGES_DIR"

# Build the basic-docker binary with error handling
echo "==== Building Project ===="
if ! go build -o basic-docker main.go image.go; then
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

# List images
echo -e "\n\n==== Listing Images ===="
sudo ./basic-docker images

# Clean up temporary directories
rm -rf "$BASE_DIR"

echo -e "\n\n==== All tests completed ===="