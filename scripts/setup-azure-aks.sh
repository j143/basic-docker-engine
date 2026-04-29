#!/usr/bin/env bash
# setup-azure-aks.sh
# Provisions Azure resources and configures GitHub secrets for the
# "Deploy and Verify on Azure AKS" workflow.
#
# Usage:
#   ./scripts/setup-azure-aks.sh [options]
#
# Options:
#   -g, --resource-group   Azure resource group name  (default: rg-basic-docker)
#   -c, --cluster          AKS cluster name           (default: basic-docker-aks)
#   -l, --location         Azure region               (default: eastus)
#   -r, --repo             GitHub repo slug            (default: j143/basic-docker-engine)
#   -b, --branch           Branch for OIDC subject     (default: main)
#   -h, --help             Show this help text

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
RESOURCE_GROUP="rg-basic-docker"
CLUSTER_NAME="basic-docker-aks"
LOCATION="eastus"
GITHUB_REPO="j143/basic-docker-engine"
BRANCH="main"
APP_NAME="basic-docker-gh-actions"

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -g|--resource-group) RESOURCE_GROUP="$2"; shift 2 ;;
    -c|--cluster)        CLUSTER_NAME="$2";   shift 2 ;;
    -l|--location)       LOCATION="$2";       shift 2 ;;
    -r|--repo)           GITHUB_REPO="$2";    shift 2 ;;
    -b|--branch)         BRANCH="$2";         shift 2 ;;
    -h|--help)
      sed -n '3,14p' "$0" | sed 's/^# \?//'
      exit 0 ;;
    *) error "Unknown option: $1" ;;
  esac
done

# ── Dependency checks ─────────────────────────────────────────────────────────
for cmd in az gh jq; do
  command -v "$cmd" &>/dev/null || error "'$cmd' is not installed. Install it and re-run."
done

# ── Azure login check ─────────────────────────────────────────────────────────
info "Checking Azure login..."
az account show &>/dev/null || az login --use-device-code
SUBSCRIPTION_ID=$(az account show --query id -o tsv)
TENANT_ID=$(az account show --query tenantId -o tsv)
success "Logged in  subscription=$SUBSCRIPTION_ID  tenant=$TENANT_ID"

# ── GitHub auth check ─────────────────────────────────────────────────────────
info "Checking GitHub CLI login..."
gh auth status &>/dev/null || gh auth login
success "GitHub CLI authenticated"

# ── Resource group ────────────────────────────────────────────────────────────
info "Ensuring resource group '$RESOURCE_GROUP' in '$LOCATION'..."
if az group show --name "$RESOURCE_GROUP" &>/dev/null; then
  warn "Resource group '$RESOURCE_GROUP' already exists — skipping creation."
else
  az group create --name "$RESOURCE_GROUP" --location "$LOCATION" --output none
  success "Resource group created."
fi

# ── AKS cluster ───────────────────────────────────────────────────────────────
info "Checking AKS cluster '$CLUSTER_NAME'..."
if az aks show --resource-group "$RESOURCE_GROUP" --name "$CLUSTER_NAME" &>/dev/null; then
  warn "AKS cluster '$CLUSTER_NAME' already exists — skipping creation."
else
  info "Creating AKS cluster (this takes ~3-5 minutes)..."
  az aks create \
    --resource-group "$RESOURCE_GROUP" \
    --name "$CLUSTER_NAME" \
    --node-count 1 \
    --node-vm-size Standard_B2s \
    --generate-ssh-keys \
    --enable-oidc-issuer \
    --enable-workload-identity \
    --output none
  success "AKS cluster created."
fi

# ── App registration ──────────────────────────────────────────────────────────
info "Ensuring app registration '$APP_NAME'..."
APP_ID=$(az ad app list --display-name "$APP_NAME" --query '[0].appId' -o tsv 2>/dev/null || true)

if [[ -z "$APP_ID" || "$APP_ID" == "None" ]]; then
  APP_ID=$(az ad app create --display-name "$APP_NAME" --query appId -o tsv)
  success "App registration created  client_id=$APP_ID"
else
  warn "App registration already exists  client_id=$APP_ID"
fi

# ── Service principal ─────────────────────────────────────────────────────────
info "Ensuring service principal..."
SP_ID=$(az ad sp list --filter "appId eq '$APP_ID'" --query '[0].id' -o tsv 2>/dev/null || true)
if [[ -z "$SP_ID" || "$SP_ID" == "None" ]]; then
  az ad sp create --id "$APP_ID" --output none
  success "Service principal created."
else
  warn "Service principal already exists."
fi

# ── Role assignment ───────────────────────────────────────────────────────────
SCOPE="/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RESOURCE_GROUP}"
info "Assigning Contributor role on resource group..."
EXISTING_ROLE=$(az role assignment list \
  --assignee "$APP_ID" \
  --role Contributor \
  --scope "$SCOPE" \
  --query '[0].id' -o tsv 2>/dev/null || true)

if [[ -z "$EXISTING_ROLE" || "$EXISTING_ROLE" == "None" ]]; then
  az role assignment create \
    --assignee "$APP_ID" \
    --role Contributor \
    --scope "$SCOPE" \
    --output none
  success "Role assigned."
else
  warn "Contributor role already assigned."
fi

# ── Federated credential ──────────────────────────────────────────────────────
FEDERATED_NAME="github-oidc-${BRANCH//\//-}"
SUBJECT="repo:${GITHUB_REPO}:ref:refs/heads/${BRANCH}"
info "Ensuring federated credential for subject '$SUBJECT'..."

EXISTING_FED=$(az ad app federated-credential list --id "$APP_ID" \
  --query "[?name=='${FEDERATED_NAME}'].name" -o tsv 2>/dev/null || true)

if [[ -z "$EXISTING_FED" ]]; then
  az ad app federated-credential create --id "$APP_ID" --parameters "{
    \"name\": \"${FEDERATED_NAME}\",
    \"issuer\": \"https://token.actions.githubusercontent.com\",
    \"subject\": \"${SUBJECT}\",
    \"audiences\": [\"api://AzureADTokenExchange\"]
  }" --output none
  success "Federated credential created."
else
  warn "Federated credential already exists."
fi

# ── GitHub secrets ────────────────────────────────────────────────────────────
# Strategy:
#   1. If GH_PAT / GITHUB_PAT env var is set, use it directly.
#   2. Otherwise try with the current token; on 403 attempt gh auth refresh
#      (opens a browser once to add the 'repo' scope), then retry.
#   3. If still failing (e.g. headless CI), print the values so they can be
#      pasted into Settings → Secrets manually.

_set_secrets() {
  local token_arg=()
  if [[ -n "${GH_PAT:-}" ]]; then
    token_arg=(--auth-token "$GH_PAT")
  elif [[ -n "${GITHUB_PAT:-}" ]]; then
    token_arg=(--auth-token "$GITHUB_PAT")
  fi

  gh secret set AZURE_CLIENT_ID       --repo "$GITHUB_REPO" --body "$APP_ID"       "${token_arg[@]+"${token_arg[@]}"}"
  gh secret set AZURE_TENANT_ID       --repo "$GITHUB_REPO" --body "$TENANT_ID"     "${token_arg[@]+"${token_arg[@]}"}"
  gh secret set AZURE_SUBSCRIPTION_ID --repo "$GITHUB_REPO" --body "$SUBSCRIPTION_ID" "${token_arg[@]+"${token_arg[@]}"}"
}

_print_manual_fallback() {
  echo ""
  warn "Could not set secrets automatically."
  warn "Go to: https://github.com/${GITHUB_REPO}/settings/secrets/actions"
  warn "and add these three secrets:"
  echo ""
  echo "  AZURE_CLIENT_ID       = $APP_ID"
  echo "  AZURE_TENANT_ID       = $TENANT_ID"
  echo "  AZURE_SUBSCRIPTION_ID = $SUBSCRIPTION_ID"
  echo ""
  warn "Or re-run with a PAT that has 'repo' scope:"
  echo "  GH_PAT=<your-pat> $0 ${*}"
  echo ""
}

info "Setting GitHub secrets on '$GITHUB_REPO'..."

if [[ -n "${GH_PAT:-}" || -n "${GITHUB_PAT:-}" ]]; then
  info "Using PAT from environment variable."
  _set_secrets && success "GitHub secrets set via PAT." || {
    warn "PAT-based secret setting failed."
    _print_manual_fallback "$@"
  }
else
  # Try with current token
  if _set_secrets 2>/dev/null; then
    success "GitHub secrets set: AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID"
  else
    info "Current token lacks 'secrets:write'. Attempting gh auth refresh..."
    if gh auth refresh --scopes "repo" 2>/dev/null; then
      if _set_secrets 2>/dev/null; then
        success "GitHub secrets set after scope refresh."
      else
        _print_manual_fallback "$@"
      fi
    else
      _print_manual_fallback "$@"
    fi
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN} Setup complete!${NC}"
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo ""
echo "  Resource group : $RESOURCE_GROUP"
echo "  AKS cluster    : $CLUSTER_NAME  (region: $LOCATION)"
echo "  Client ID      : $APP_ID"
echo "  Tenant ID      : $TENANT_ID"
echo "  Subscription   : $SUBSCRIPTION_ID"
echo ""
echo "Next step — trigger the workflow:"
echo ""
echo "  gh workflow run azure-aks-verify.yml \\"
echo "    --repo $GITHUB_REPO \\"
echo "    --field resource_group=$RESOURCE_GROUP \\"
echo "    --field aks_cluster=$CLUSTER_NAME"
echo ""
