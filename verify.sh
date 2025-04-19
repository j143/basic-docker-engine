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

# Function to clean up resources on exit
cleanup() {
  echo "Cleaning up..."
  docker stop registry &>/dev/null
  docker rm registry &>/dev/null
  rm -rf auth
  echo "Cleanup completed."
}

# Trap to ensure cleanup on script exit
trap cleanup EXIT

# Step 1: Start a local Docker registry with authentication
echo "Starting local Docker registry with authentication..."
mkdir -p auth
if ! docker run --entrypoint htpasswd httpd:2 -Bbn user password > auth/htpasswd; then
    echo "Error: Failed to create htpasswd file." >&2
    exit 1
fi
docker run -d -p 5000:5000 --name registry \
  -v $(pwd)/auth:/auth \
  -e "REGISTRY_AUTH=htpasswd" \
  -e "REGISTRY_AUTH_HTPASSWD_REALM=Registry Realm" \
  -e "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd" \
  registry:2

# Step 2: Ensure the alpine:latest image is pulled before tagging and pushing
if ! docker pull alpine:latest; then
    echo "Error: Failed to pull alpine:latest image." >&2
    exit 1
fi

# Use echo with --password-stdin for secure login
echo "password" | docker login localhost:5000 -u user --password-stdin

# Tag and push the image to the local registry
docker tag alpine:latest localhost:5000/alpine
if ! docker push localhost:5000/alpine; then
    echo "Error: Failed to push the alpine:latest image to the local registry." >&2
    docker logs registry >&2
    exit 1
fi

# Debugging: Check if the registry is running
if ! docker ps | grep -q registry; then
    echo "Error: Registry container is not running." >&2
    docker logs registry >&2
    exit 1
fi

# Debugging: Check registry logs before pushing
echo "Checking registry logs before pushing the image..."
docker logs registry

# Debugging: Verify registry accessibility
if ! curl -s -u user:password -X GET http://localhost:5000/v2/ > /dev/null; then
    echo "Error: Unable to access the registry at localhost:5000." >&2
    exit 1
fi

# Step 3: Verify the image in the local registry
echo "Verifying the image in the local registry..."
catalog=$(curl -s -u user:password -X GET http://localhost:5000/v2/_catalog)
echo "Registry catalog: $catalog"

# Step 4: Use basic-docker to pull the image from the local registry
if ! ./basic-docker pull user:password@localhost:5000/alpine; then
    echo "Error: basic-docker failed to pull the image from the local registry." >&2
    exit 1
fi

# Step 5: Use basic-docker to run the image
if ./basic-docker run user:password@localhost:5000/alpine /bin/sh -c "echo Hello from authenticated local registry"; then
    echo "basic-docker successfully pulled and ran the image."
else
    echo "Error: basic-docker failed to run the image." >&2
    exit 1
fi

# Step 6: Check logs for authentication
echo "Checking logs for authentication..."
docker logs registry | grep "user"

echo "Script completed successfully."

# Clean up temporary directories
rm -rf "$BASE_DIR"

echo -e "\n\n==== All tests completed ===="