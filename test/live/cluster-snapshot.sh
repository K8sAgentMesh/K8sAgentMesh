#!/usr/bin/env bash
# cluster-snapshot.sh <outdir> — capture the cluster-wide control-plane
# state the zero-control-plane assertion runs against: namespaces, CRDs,
# cluster RBAC, admission webhooks, and the api-resources inventory.
set -euo pipefail
out="$1"
mkdir -p "$out"

kubectl get namespaces --no-headers | awk '{print $1}' | sort > "$out/namespaces.txt"
kubectl get crd --no-headers | awk '{print $1}' | sort > "$out/crds.txt"
kubectl get clusterroles --no-headers | awk '{print $1}' | sort > "$out/clusterroles.txt"
kubectl get clusterrolebindings --no-headers | awk '{print $1}' | sort > "$out/clusterrolebindings.txt"
kubectl get mutatingwebhookconfigurations,validatingwebhookconfigurations --no-headers 2>/dev/null \
  | awk '{print $1}' | sort > "$out/webhooks.txt" || true
kubectl api-resources --namespaced=false -o name | sort > "$out/cluster-scoped-kinds.txt"

echo "snapshot written to $out"
