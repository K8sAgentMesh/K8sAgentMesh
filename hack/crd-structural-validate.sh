#!/usr/bin/env bash
# crd-structural-validate.sh — the CRD contract guard (P3.8):
# the mesh-owned CRD YAML under apis/mesh/v1alpha1/crd must pass
# structural-schema validation and stay consistent with the Go types.
#
# Checks:
#   1. CRD object shape: apiextensions.k8s.io/v1, metadata.name = <plural>.<group>,
#      group/kind/listKind agree with the Go package constants.
#   2. Structural schema: every schema node under openAPIV3Schema carries a
#      type; objects own properties/additionalProperties; arrays own items;
#      primitives own none of them; every `required` entry exists.
#   3. Go<->YAML drift guard: every property field name appears as a JSON
#      tag in the package's Go sources (extraction-clean applies to CRD
#      shapes exactly as to imports).
#
# kubectl client dry-run is deliberately NOT used: it requires cluster
# discovery even for --dry-run=client, which CI runners do not have. The
# structural walk above is the deterministic, offline check.
#
# Exit 0 = valid; exit 1 = violation (printed).
set -euo pipefail

cd "$(dirname "$0")/.."

if ! python3 -c "import yaml" 2>/dev/null; then
  echo "FAIL: python3 with PyYAML is required (pip install pyyaml)."
  exit 1
fi

python3 - <<'PY'
import glob
import re
import sys

import yaml

CRD_DIR = "apis/mesh/v1alpha1/crd"
GO_SRC = ""
go_text = ""
for path in glob.glob("apis/mesh/v1alpha1/*.go"):
    with open(path) as f:
        go_text += f.read()

# Group / kind constants from meta.go and the *_types.go files.
def go_const(name):
    m = re.search(rf'\b{name}\s+=\s+"([^"]+)"', go_text)
    return m.group(1) if m else None

GROUP = go_const("GroupName")
KINDS = {
    go_const("KindMeshPool"): "meshpools",
    go_const("KindMeshRoute"): "meshroutes",
    go_const("KindMeshTelemetry"): "meshtelemetries",
}

errors = []


def err(msg):
    errors.append(msg)


def check_schema(node, path, seen):
    """Enforce structural-schema invariants on one schema node."""
    if not isinstance(node, dict):
        err(f"{path}: schema node must be a mapping, got {type(node).__name__}")
        return
    if "$ref" in node:
        err(f"{path}: $ref is not allowed in structural schemas (inline it)")
        return
    ntype = node.get("type")
    if not ntype:
        err(f"{path}: missing 'type' (structural schemas require a type at every node)")
        return
    seen[0] += 1
    has_props = "properties" in node
    has_addl = isinstance(node.get("additionalProperties"), dict)
    has_items = "items" in node
    preserve = node.get("x-kubernetes-preserve-unknown-fields", False)
    if ntype == "object":
        if has_items:
            err(f"{path}: object schema must not carry 'items'")
        if not (has_props or has_addl or preserve):
            err(f"{path}: object schema needs 'properties' or 'additionalProperties' "
                f"(or x-kubernetes-preserve-unknown-fields)")
        for req in node.get("required", []):
            if req not in (node.get("properties") or {}):
                err(f"{path}: required entry '{req}' has no property schema")
        for name, child in (node.get("properties") or {}).items():
            check_schema(child, f"{path}.{name}", seen)
        if has_addl:
            check_schema(node["additionalProperties"], f"{path}.<additionalProperties>", seen)
    elif ntype == "array":
        if has_props or has_addl:
            err(f"{path}: array schema must not carry 'properties'/'additionalProperties'")
        if "items" not in node:
            err(f"{path}: array schema needs 'items'")
        else:
            check_schema(node["items"], f"{path}[]", seen)
    else:  # string / integer / number / boolean
        if has_props or has_addl or has_items:
            err(f"{path}: {ntype} schema must not carry 'properties'/'items'/'additionalProperties'")
        enum = node.get("enum")
        if enum is not None:
            for v in enum:
                if not isinstance(v, (str, int, float, bool)):
                    err(f"{path}: enum values must be scalars")


def check_crd(path, doc):
    if not isinstance(doc, dict):
        err(f"{path}: top level is not a mapping")
        return
    if doc.get("apiVersion") != "apiextensions.k8s.io/v1":
        err(f"{path}: apiVersion must be apiextensions.k8s.io/v1")
        return
    if doc.get("kind") != "CustomResourceDefinition":
        err(f"{path}: kind must be CustomResourceDefinition")
        return
    spec = doc.get("spec") or {}
    group = spec.get("group")
    if group != GROUP:
        err(f"{path}: spec.group {group!r} != Go GroupName {GROUP!r}")
    names = spec.get("names") or {}
    kind = names.get("kind")
    plural = names.get("plural")
    if kind not in KINDS:
        err(f"{path}: kind {kind!r} is not a published Kind constant")
    else:
        if plural != KINDS[kind]:
            err(f"{path}: plural {plural!r} does not match kind {kind!r}")
        if names.get("listKind") != kind + "List":
            err(f"{path}: listKind must be {kind}List")
    expected_name = f"{plural}.{group}"
    if (doc.get("metadata") or {}).get("name") != expected_name:
        err(f"{path}: metadata.name must be {expected_name}")
    versions = spec.get("versions") or []
    if not versions:
        err(f"{path}: no versions")
    for v in versions:
        if v.get("name") != "v1alpha1":
            err(f"{path}: unexpected version {v.get('name')!r}")
        if not v.get("served") or not v.get("storage"):
            err(f"{path}: v1alpha1 must be served and the storage version")
        schema = (v.get("schema") or {}).get("openAPIV3Schema")
        if not schema:
            err(f"{path}: missing openAPIV3Schema")
            continue
        seen = [0]
        check_schema(schema, f"{path}::schema", seen)
        # Go<->YAML drift guard: every spec property name must exist as a
        # JSON tag in the package's Go sources. Root apiVersion/kind/
        # metadata are inline TypeMeta/ObjectMeta tags; check them too.
        props = ((schema.get("properties") or {}).get("spec") or {}).get("properties") or {}
        for name in props:
            if f'json:"{name}' not in go_text:
                err(f"{path}: spec property '{name}' has no JSON tag in the Go types (drift)")
        for root in ("apiVersion", "kind", "metadata"):
            if f'json:"{root}' not in go_text:
                err(f"{path}: root '{root}' has no JSON tag in the Go types (drift)")
        print(f"  {path}: {seen[0]} schema nodes OK")


files = sorted(glob.glob(f"{CRD_DIR}/*.yaml"))
if len(files) != 3:
    err(f"expected exactly 3 CRD files in {CRD_DIR}, found {len(files)}: {files}")
for path in files:
    with open(path) as f:
        try:
            docs = list(yaml.safe_load_all(f))
        except yaml.YAMLError as e:
            err(f"{path}: YAML parse error: {e}")
            continue
    for doc in docs:
        if doc is not None:
            check_crd(path, doc)

if errors:
    print("FAIL: CRD structural-schema violations:")
    for e in errors:
        print(f"  - {e}")
    sys.exit(1)
print("OK: CRD YAML is structurally valid and consistent with the Go types.")
PY
