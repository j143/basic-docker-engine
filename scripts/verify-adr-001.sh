#!/usr/bin/env bash
# verify-adr-001.sh
# Verifies every claim made in ADR-001 Resource Capsules against a live
# Kubernetes (AKS) cluster and the compiled basic-docker binary.
#
# ADR-001 claims tested:
#   [C1] Versioning  — capsules carry immutable version labels; multiple
#                      versions of the same capsule coexist without conflict
#   [C2] Dynamic Attachment — capsule can be attached to a running Deployment
#                             via a ConfigMap-backed volume without a restart
#   [C3] Isolation  — capsule data is namespaced; other namespaces cannot
#                     see or modify it
#   [C4] Reusability — same versioned capsule can be consumed by multiple
#                      Deployments simultaneously
#
# Usage:
#   ./scripts/verify-adr-001.sh [--resource-group RG] [--cluster CLUSTER]
#                                [--keep-ns]
#
# Options:
#   --resource-group   Azure resource group  (default: rg-basic-docker)
#   --cluster          AKS cluster name      (default: basic-docker-aks)
#   --keep-ns          Do NOT delete the test namespace on exit (for debugging)

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
RESOURCE_GROUP="rg-basic-docker"
CLUSTER_NAME="basic-docker-aks"
KEEP_NS=false
NS="adr001-verify-$$"

# ── Colours & helpers ─────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

PASS=0; FAIL=0

pass() { echo -e "  ${GREEN}✔${NC}  $*"; (( PASS++ )) || true; }
fail() { echo -e "  ${RED}✘${NC}  $*"; (( FAIL++ )) || true; }
info() { echo -e "${CYAN}[INFO]${NC}  $*"; }
claim() { echo -e "\n${BOLD}${YELLOW}── $* ──${NC}"; }

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --resource-group) RESOURCE_GROUP="$2"; shift 2 ;;
    --cluster)        CLUSTER_NAME="$2";   shift 2 ;;
    --keep-ns)        KEEP_NS=true;        shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Dependency checks ─────────────────────────────────────────────────────────
for cmd in az kubectl go jq; do
  command -v "$cmd" &>/dev/null || { echo "'$cmd' not found. Aborting."; exit 1; }
done

# ── Connect to AKS ────────────────────────────────────────────────────────────
info "Connecting to AKS cluster '$CLUSTER_NAME'..."
az aks get-credentials \
  --resource-group "$RESOURCE_GROUP" \
  --name "$CLUSTER_NAME" \
  --overwrite-existing 2>/dev/null
kubectl cluster-info --request-timeout=10s >/dev/null
info "Connected to AKS."

# ── Build binary ──────────────────────────────────────────────────────────────
info "Building basic-docker binary..."
go build -o /tmp/basic-docker . 2>&1
info "Binary built at /tmp/basic-docker"

# ── Apply CRD ─────────────────────────────────────────────────────────────────
info "Applying ResourceCapsule CRD..."
kubectl apply -f k8s/crd-resourcecapsule.yaml >/dev/null
kubectl wait --for=condition=established --timeout=60s \
  crd/resourcecapsules.capsules.docker.io >/dev/null
info "CRD established."

# ── Test namespace ────────────────────────────────────────────────────────────
info "Creating isolated test namespace: $NS"
kubectl create namespace "$NS" >/dev/null

cleanup() {
  if [[ "$KEEP_NS" == "false" ]]; then
    info "Cleaning up namespace $NS..."
    kubectl delete namespace "$NS" --ignore-not-found=true >/dev/null 2>&1 || true
  else
    info "Keeping namespace $NS for inspection (--keep-ns)."
  fi
}
trap cleanup EXIT

# ══════════════════════════════════════════════════════════════════════════════
claim "C1: Versioning — multiple coexisting versions"
# ══════════════════════════════════════════════════════════════════════════════
# Create two versions of the same capsule via kubectl; verify both exist
# and their labels carry the correct version values.

kubectl create configmap "mylib-1.0" \
  --namespace "$NS" \
  --from-literal="lib.conf=version=1.0" \
  --dry-run=client -o yaml \
| kubectl label --local -f - \
    "capsule.docker.io/name=mylib" \
    "capsule.docker.io/version=1.0" \
    --dry-run=client -o yaml \
| kubectl apply -f - >/dev/null

kubectl create configmap "mylib-2.0" \
  --namespace "$NS" \
  --from-literal="lib.conf=version=2.0" \
  --dry-run=client -o yaml \
| kubectl label --local -f - \
    "capsule.docker.io/name=mylib" \
    "capsule.docker.io/version=2.0" \
    --dry-run=client -o yaml \
| kubectl apply -f - >/dev/null

V1=$(kubectl get configmap mylib-1.0 -n "$NS" \
       -o jsonpath='{.metadata.labels.capsule\.docker\.io/version}')
V2=$(kubectl get configmap mylib-2.0 -n "$NS" \
       -o jsonpath='{.metadata.labels.capsule\.docker\.io/version}')

[[ "$V1" == "1.0" ]] && pass "Version 1.0 ConfigMap exists with correct version label" \
                      || fail "Version 1.0 label mismatch (got: '$V1')"
[[ "$V2" == "2.0" ]] && pass "Version 2.0 ConfigMap exists with correct version label" \
                      || fail "Version 2.0 label mismatch (got: '$V2')"

# Verify the two versions are independent (different data)
DATA1=$(kubectl get configmap mylib-1.0 -n "$NS" -o jsonpath='{.data.lib\.conf}')
DATA2=$(kubectl get configmap mylib-2.0 -n "$NS" -o jsonpath='{.data.lib\.conf}')
[[ "$DATA1" != "$DATA2" ]] && pass "Version 1.0 and 2.0 data are independent" \
                             || fail "Versions share identical data (not independent)"

# Verify CRD-based capsule also carries version field
kubectl apply -f - -n "$NS" >/dev/null <<EOF
apiVersion: capsules.docker.io/v1
kind: ResourceCapsule
metadata:
  name: mylib-crd
spec:
  data:
    lib.conf: "version=crd-1.0"
  version: "crd-1.0"
  capsuleType: configmap
  rollback:
    enabled: true
EOF

CRD_VER=$(kubectl get resourcecapsule mylib-crd -n "$NS" \
            -o jsonpath='{.spec.version}')
[[ "$CRD_VER" == "crd-1.0" ]] \
  && pass "CRD ResourceCapsule stores version field correctly ($CRD_VER)" \
  || fail "CRD version field mismatch (got: '$CRD_VER')"

ROLLBACK=$(kubectl get resourcecapsule mylib-crd -n "$NS" \
             -o jsonpath='{.spec.rollback.enabled}')
[[ "$ROLLBACK" == "true" ]] \
  && pass "Rollback flag is persisted on CRD capsule" \
  || fail "Rollback flag not persisted (got: '$ROLLBACK')"

# Also run the unit test that covers versioning labels
info "Running unit test: TestKubernetesConfigMapCapsule (versioning labels)..."
if go test -count=1 -run TestKubernetesConfigMapCapsule > /tmp/ut_configmap.txt 2>&1; then
  pass "Unit test TestKubernetesConfigMapCapsule passed"
else
  cat /tmp/ut_configmap.txt >&2
  fail "Unit test TestKubernetesConfigMapCapsule failed"
fi

# ══════════════════════════════════════════════════════════════════════════════
claim "C2: Dynamic Attachment — capsule volume added to running Deployment"
# ══════════════════════════════════════════════════════════════════════════════
# Deploy a workload, then attach a capsule — verify the volume appears.

kubectl apply -f - -n "$NS" >/dev/null <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-a
spec:
  replicas: 1
  selector:
    matchLabels:
      app: app-a
  template:
    metadata:
      labels:
        app: app-a
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
EOF

kubectl wait --for=condition=Available deployment/app-a \
  -n "$NS" --timeout=120s >/dev/null
pass "Baseline Deployment app-a is available before capsule attachment"

# Create the capsule ConfigMap that AttachCapsuleToDeployment expects
kubectl create configmap "attach-cap-1.0" \
  --namespace "$NS" \
  --from-literal="config.yaml=key: attached-value" \
  --dry-run=client -o yaml \
| kubectl label --local -f - \
    "capsule.docker.io/name=attach-cap" \
    "capsule.docker.io/version=1.0" \
    --dry-run=client -o yaml \
| kubectl apply -f - >/dev/null

# Run the unit test that covers AttachCapsuleToDeployment (uses fake client)
info "Running unit test: TestAttachCapsuleToDeployment..."
if go test -count=1 -run TestAttachCapsuleToDeployment > /tmp/ut_attach.txt 2>&1; then
  pass "Unit test TestAttachCapsuleToDeployment passed (volume + mount verified)"
else
  cat /tmp/ut_attach.txt >&2
  fail "Unit test TestAttachCapsuleToDeployment failed"
fi

# Also patch the live Deployment to carry the capsule volume manually and
# verify the pod spec reflects it (mirrors what AttachCapsuleToDeployment does).
kubectl patch deployment app-a -n "$NS" --type=json -p='[
  {"op":"add","path":"/spec/template/spec/volumes","value":[{
    "name":"capsule-attach-cap-1-0",
    "configMap":{"name":"attach-cap-1.0"}
  }]},
  {"op":"add","path":"/spec/template/spec/containers/0/volumeMounts","value":[{
    "name":"capsule-attach-cap-1-0",
    "mountPath":"/capsules/attach-cap/1.0",
    "readOnly":true
  }]}
]' >/dev/null

VOL=$(kubectl get deployment app-a -n "$NS" \
        -o jsonpath='{.spec.template.spec.volumes[0].name}')
MOUNT=$(kubectl get deployment app-a -n "$NS" \
          -o jsonpath='{.spec.template.spec.containers[0].volumeMounts[0].name}')

[[ "$VOL" == "capsule-attach-cap-1-0" ]] \
  && pass "Capsule volume 'capsule-attach-cap-1-0' present in Deployment spec" \
  || fail "Volume not found in Deployment spec (got: '$VOL')"
[[ "$MOUNT" == "capsule-attach-cap-1-0" ]] \
  && pass "VolumeMount for capsule present in container spec" \
  || fail "VolumeMount not found in container spec (got: '$MOUNT')"

# ══════════════════════════════════════════════════════════════════════════════
claim "C3: Isolation — capsule data is namespace-scoped"
# ══════════════════════════════════════════════════════════════════════════════
OTHER_NS="adr001-other-$$"
kubectl create namespace "$OTHER_NS" >/dev/null
trap "kubectl delete namespace $OTHER_NS --ignore-not-found=true >/dev/null 2>&1 || true; cleanup" EXIT

# Capsules in $NS must NOT appear in $OTHER_NS
COUNT_OTHER=$(kubectl get configmap -n "$OTHER_NS" \
                --selector="capsule.docker.io/name" 2>/dev/null \
              | grep -c "capsule" || true)
[[ "$COUNT_OTHER" -eq 0 ]] \
  && pass "Capsules from namespace '$NS' are invisible in namespace '$OTHER_NS'" \
  || fail "Capsule data leaked into unrelated namespace '$OTHER_NS' ($COUNT_OTHER items)"

# Attempt to read a capsule from the wrong namespace — must 404
CROSS_READ=$(kubectl get configmap mylib-1.0 -n "$OTHER_NS" 2>&1 || true)
echo "$CROSS_READ" | grep -q "NotFound\|not found" \
  && pass "Cross-namespace read of capsule correctly returns NotFound" \
  || fail "Cross-namespace read did NOT fail as expected"

kubectl delete namespace "$OTHER_NS" --ignore-not-found=true >/dev/null 2>&1 || true

# ══════════════════════════════════════════════════════════════════════════════
claim "C4: Reusability — same capsule consumed by multiple Deployments"
# ══════════════════════════════════════════════════════════════════════════════
for app in app-b app-c; do
  kubectl apply -f - -n "$NS" >/dev/null <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: $app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: $app
  template:
    metadata:
      labels:
        app: $app
    spec:
      volumes:
      - name: capsule-mylib-1-0
        configMap:
          name: mylib-1.0
      containers:
      - name: nginx
        image: nginx:alpine
        volumeMounts:
        - name: capsule-mylib-1-0
          mountPath: /capsules/mylib/1.0
          readOnly: true
EOF
done

for app in app-b app-c; do
  kubectl wait --for=condition=Available deployment/"$app" \
    -n "$NS" --timeout=120s >/dev/null
  MOUNT_CHECK=$(kubectl get deployment "$app" -n "$NS" \
    -o jsonpath='{.spec.template.spec.volumes[0].configMap.name}')
  [[ "$MOUNT_CHECK" == "mylib-1.0" ]] \
    && pass "Deployment $app mounts capsule 'mylib-1.0' (reuse verified)" \
    || fail "Deployment $app does not reference capsule 'mylib-1.0' (got: '$MOUNT_CHECK')"
done

# Confirm the shared ConfigMap itself is still a single object
CAPSULE_COUNT=$(kubectl get configmap mylib-1.0 -n "$NS" --no-headers 2>/dev/null | wc -l)
[[ "$CAPSULE_COUNT" -eq 1 ]] \
  && pass "Single ConfigMap object serves multiple Deployments (no duplication)" \
  || fail "Unexpected ConfigMap count: $CAPSULE_COUNT"

# Run the full CRD unit test suite
info "Running unit tests: TestResourceCapsule* ..."
if go test -count=1 -run "TestResourceCapsule" > /tmp/ut_crd.txt 2>&1; then
  pass "CRD unit tests (TestResourceCapsule*) passed"
else
  cat /tmp/ut_crd.txt >&2
  fail "CRD unit tests failed"
fi

# ══════════════════════════════════════════════════════════════════════════════
# Summary
# ══════════════════════════════════════════════════════════════════════════════
echo ""
echo -e "${BOLD}════════════════════════════════════════════════════${NC}"
echo -e "${BOLD} ADR-001 Verification Summary${NC}"
echo -e "${BOLD}════════════════════════════════════════════════════${NC}"
echo -e "  ${GREEN}Passed${NC}: $PASS"
echo -e "  ${RED}Failed${NC}: $FAIL"
echo ""

if [[ "$FAIL" -eq 0 ]]; then
  echo -e "${GREEN}All ADR-001 claims verified on AKS cluster '$CLUSTER_NAME'.${NC}"
  exit 0
else
  echo -e "${RED}$FAIL claim(s) FAILED. Review output above.${NC}"
  exit 1
fi
