#!/bin/bash

# Example: Resource Capsules with CRDs and Operator Demo

set -e

echo "🚀 Resource Capsules CRD and Operator Demo"
echo "=========================================="

# Step 1: Install the CRD
echo "📋 Step 1: Installing ResourceCapsule CRD..."
kubectl apply -f k8s/crd-resourcecapsule.yaml
kubectl wait --for condition=established --timeout=30s crd/resourcecapsules.capsules.docker.io
echo "✅ CRD installed successfully"

# Step 2: Create a test namespace
echo "📦 Step 2: Creating demo namespace..."
kubectl create namespace demo || echo "Namespace already exists"
echo "✅ Namespace ready"

# Step 3: Create sample configuration file
echo "📝 Step 3: Creating sample configuration..."
mkdir -p /tmp/demo-configs
cat > /tmp/demo-configs/app-config.yaml << EOF
database:
  host: postgres.demo.svc.cluster.local
  port: 5432
  name: myapp
redis:
  host: redis.demo.svc.cluster.local
  port: 6379
features:
  auth: enabled
  cache: enabled
  logging: debug
EOF

# Step 4: Create ResourceCapsule using CLI
echo "🔧 Step 4: Creating ResourceCapsule via CLI..."
basic-docker k8s-crd create app-config 1.0 /tmp/demo-configs/app-config.yaml configmap

# Step 5: Create ResourceCapsule using kubectl
echo "🔧 Step 5: Creating ResourceCapsule via kubectl..."
cat << EOF | kubectl apply -f - -n demo
apiVersion: capsules.docker.io/v1
kind: ResourceCapsule
metadata:
  name: database-config
spec:
  data:
    database.yaml: |
      host: db.example.com
      port: 5432
      ssl: true
      pool_size: 20
  version: "1.0"
  capsuleType: secret
  rollback:
    enabled: true
EOF

# Step 6: List ResourceCapsules
echo "📋 Step 6: Listing ResourceCapsules..."
kubectl get resourcecapsules -n demo
basic-docker k8s-crd list

# Step 7: Show ResourceCapsule details
echo "🔍 Step 7: Showing ResourceCapsule details..."
kubectl describe resourcecapsule database-config -n demo

# Step 8: Start operator (in background for demo)
echo "🤖 Step 8: Starting ResourceCapsule operator..."
basic-docker k8s-crd operator start demo &
OPERATOR_PID=$!
sleep 5

# Step 9: Update ResourceCapsule (triggers operator)
echo "🔄 Step 9: Updating ResourceCapsule..."
kubectl patch resourcecapsule database-config -n demo --type merge -p '
{
  "spec": {
    "data": {
      "database.yaml": "host: db.example.com\nport: 5432\nssl: true\npool_size: 30\nmax_connections: 100"
    },
    "version": "1.1"
  }
}'

# Wait for operator to process
sleep 3

# Step 10: Check created resources
echo "📦 Step 10: Checking created ConfigMaps and Secrets..."
kubectl get configmaps -n demo --selector="capsule.docker.io/managed-by=resourcecapsule-operator"
kubectl get secrets -n demo --selector="capsule.docker.io/managed-by=resourcecapsule-operator"

# Step 11: Test rollback functionality
echo "🔄 Step 11: Testing rollback functionality..."
basic-docker k8s-crd rollback database-config 1.0

# Step 12: Clean up
echo "🧹 Step 12: Cleaning up..."
kill $OPERATOR_PID 2>/dev/null || true
kubectl delete resourcecapsules --all -n demo
kubectl delete namespace demo
kubectl delete crd resourcecapsules.capsules.docker.io
rm -rf /tmp/demo-configs

echo "✅ Demo completed successfully!"
echo ""
echo "🎉 ResourceCapsule CRD and Operator Demo Features Demonstrated:"
echo "   • CRD installation and management"
echo "   • CLI integration for ResourceCapsule management"
echo "   • Operator-based automated resource creation"
echo "   • Version management and rollback capabilities"
echo "   • Integration with native Kubernetes resources"
echo "   • GitOps-ready declarative configuration"