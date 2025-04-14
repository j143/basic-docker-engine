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
if ! go build -o basic-docker main.go network.go image.go; then
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

# Test networking functionality

# Create a network
echo -e "\n\n==== Creating Network ===="
./basic-docker network-create test-network

# List networks
echo -e "\n\n==== Listing Networks ===="
./basic-docker network-list

# Delete the network
echo -e "\n\n==== Deleting Network ===="
./basic-docker network-delete net-1

# List networks again
echo -e "\n\n==== Listing Networks After Deletion ===="
./basic-docker network-list

# Clean up temporary directories
rm -rf "$BASE_DIR"

echo -e "\n\n==== All tests completed ===="