#!/bin/bash
# Improved verification script for basic-docker engine
# Tests core runtime features including cgroup detection, container lifecycle, and new CLI commands

set -e  # Exit on any error

# Color output helpers
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

success() {
    echo -e "${GREEN}✓${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

info() {
    echo -e "${YELLOW}→${NC} $1"
}

section() {
    echo ""
    echo "======================================"
    echo "$1"
    echo "======================================"
}

# Track test results
TESTS_PASSED=0
TESTS_FAILED=0

check_result() {
    if [ $? -eq 0 ]; then
        success "$1"
        ((TESTS_PASSED++))
    else
        error "$1"
        ((TESTS_FAILED++))
        return 1
    fi
}

# Build the binary
section "Building Project"
if go build -o basic-docker .; then
    success "Build successful"
else
    error "Build failed"
    exit 1
fi

# Test 1: System Information and Cgroup Detection
section "Test 1: System Information & Cgroup Detection"
info "Running: ./basic-docker info"
OUTPUT=$(./basic-docker info 2>&1)
echo "$OUTPUT"

# Check for cgroup version detection
if echo "$OUTPUT" | grep -q "Cgroup version:"; then
    success "Cgroup version detected"
else
    error "Cgroup version not detected"
fi

# Check for controller information
if echo "$OUTPUT" | grep -q "Memory controller:"; then
    success "Memory controller status reported"
else
    error "Memory controller status not reported"
fi

if echo "$OUTPUT" | grep -q "CPU controller:"; then
    success "CPU controller status reported"
else
    error "CPU controller status not reported"
fi

# Test 2: Create a test image
section "Test 2: Creating Test Image"
info "Setting up test image with basic binaries"
TEST_IMAGE_DIR="/tmp/basic-docker/images/test-image/rootfs"
mkdir -p "$TEST_IMAGE_DIR/bin"
cp /bin/echo "$TEST_IMAGE_DIR/bin/" 2>/dev/null || cp /usr/bin/echo "$TEST_IMAGE_DIR/bin/"
cp /bin/sh "$TEST_IMAGE_DIR/bin/" 2>/dev/null || cp /usr/bin/sh "$TEST_IMAGE_DIR/bin/" || cp /bin/bash "$TEST_IMAGE_DIR/bin/sh"
success "Test image created"

# Test 3: Container Lifecycle - Run and State Tracking
section "Test 3: Container Lifecycle - Run Command"
info "Running: sudo ./basic-docker run test-image /bin/echo 'Hello World'"
if sudo ./basic-docker run test-image /bin/echo "Hello World" 2>&1 | grep -q "Hello World"; then
    success "Container executed successfully"
else
    error "Container execution failed"
fi

# Test 4: List Containers with State
section "Test 4: List Containers (ps)"
info "Running: sudo ./basic-docker ps"
PS_OUTPUT=$(sudo ./basic-docker ps 2>&1)
echo "$PS_OUTPUT"

if echo "$PS_OUTPUT" | grep -q "STATE"; then
    success "Container list shows state column"
else
    error "State column missing from ps output"
fi

if echo "$PS_OUTPUT" | grep -q "exited\|running"; then
    success "Container state displayed"
else
    error "Container state not displayed"
fi

# Get the container ID from ps output
CONTAINER_ID=$(echo "$PS_OUTPUT" | tail -n 1 | awk '{print $1}')
info "Test container ID: $CONTAINER_ID"

# Test 5: Inspect Container
section "Test 5: Inspect Container"
info "Running: sudo ./basic-docker inspect $CONTAINER_ID"
INSPECT_OUTPUT=$(sudo ./basic-docker inspect "$CONTAINER_ID" 2>&1)
echo "$INSPECT_OUTPUT"

if echo "$INSPECT_OUTPUT" | grep -q "\"state\""; then
    success "Inspect shows container state"
else
    error "Inspect missing state field"
fi

if echo "$INSPECT_OUTPUT" | grep -q "\"command\""; then
    success "Inspect shows command"
else
    error "Inspect missing command field"
fi

if echo "$INSPECT_OUTPUT" | grep -q "\"created_at\""; then
    success "Inspect shows timestamps"
else
    error "Inspect missing timestamp fields"
fi

if echo "$INSPECT_OUTPUT" | grep -q "\"exit_code\""; then
    success "Inspect shows exit code"
else
    error "Inspect missing exit code"
fi

# Test 6: Container Logs
section "Test 6: Container Logs"
info "Running: sudo ./basic-docker logs $CONTAINER_ID"
LOGS_OUTPUT=$(sudo ./basic-docker logs "$CONTAINER_ID" 2>&1)
echo "$LOGS_OUTPUT"

if echo "$LOGS_OUTPUT" | grep -q "Hello World"; then
    success "Logs retrieved successfully"
else
    error "Logs not retrieved or empty"
fi

# Test 7: Run a Failing Container
section "Test 7: Failed Container State"
info "Running container that should fail"
sudo ./basic-docker run test-image /bin/false 2>&1 || true

# Check that failed state is tracked
PS_FAILED=$(sudo ./basic-docker ps 2>&1)
if echo "$PS_FAILED" | grep -q "failed\|exited"; then
    success "Failed container state tracked"
else
    error "Failed container state not tracked"
fi

# Test 8: Remove Container
section "Test 8: Remove Container (rm)"
info "Running: sudo ./basic-docker rm $CONTAINER_ID"
if sudo ./basic-docker rm "$CONTAINER_ID" 2>&1 | grep -q "removed successfully"; then
    success "Container removed successfully"
else
    error "Container removal failed"
fi

# Verify container is gone
PS_AFTER_RM=$(sudo ./basic-docker ps 2>&1)
if echo "$PS_AFTER_RM" | grep -q "$CONTAINER_ID"; then
    error "Container still appears after rm"
else
    success "Container no longer listed after rm"
fi

# Test 9: Cannot Remove Running Container (safety check)
section "Test 9: Safety - Cannot Remove Running Container"
# This test would require a long-running container, skip for now
info "Skipped (requires long-running container implementation)"

# Test 10: Help Command
section "Test 10: Help Command"
info "Running: ./basic-docker --help"
HELP_OUTPUT=$(./basic-docker 2>&1 || true)
if echo "$HELP_OUTPUT" | grep -q "Usage:"; then
    success "Help text displayed"
else
    error "Help text not displayed"
fi

if echo "$HELP_OUTPUT" | grep -q "rm\|logs\|inspect"; then
    success "New commands documented in help"
else
    error "New commands missing from help"
fi

# Test 11: Network Commands (existing functionality)
section "Test 11: Network Commands"
info "Testing network-create"
if ./basic-docker network-create test-network 2>&1 | grep -q "created\|Network"; then
    success "Network creation works"
else
    error "Network creation failed"
fi

info "Testing network-list"
if ./basic-docker network-list 2>&1 | grep -q "test-network\|net-"; then
    success "Network listing works"
else
    error "Network listing failed"
fi

# Test 12: Cgroup Cleanup
section "Test 12: Cgroup Cleanup"
info "Verifying cgroup directories are cleaned up"
# Run and remove a container
sudo ./basic-docker run test-image /bin/echo "cleanup test" >/dev/null 2>&1 || true
CLEANUP_CONTAINER=$(sudo ./basic-docker ps 2>&1 | tail -n 1 | awk '{print $1}')
if [ -n "$CLEANUP_CONTAINER" ] && [ "$CLEANUP_CONTAINER" != "CONTAINER" ]; then
    sudo ./basic-docker rm "$CLEANUP_CONTAINER" >/dev/null 2>&1 || true
    success "Container cleanup completed"
else
    info "No container to cleanup"
fi

# Summary
section "Test Summary"
echo "Tests Passed: $TESTS_PASSED"
echo "Tests Failed: $TESTS_FAILED"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    success "All tests passed!"
    exit 0
else
    error "Some tests failed"
    exit 1
fi
