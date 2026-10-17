/*
Copyright 2026 The K8sAgentMesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"
	"time"
)

// GroupName is the API group the mesh-owned CRDs live under. It is
// deliberately not a host domain: hosts adopt the CRD set, they do not
// re-own it.
const GroupName = "mesh.k8sagentmesh.io"

// Version is the served version of the CRD contracts.
const Version = "v1alpha1"

// GroupVersion is group/version in apiVersion form.
const GroupVersion = GroupName + "/" + Version

// Kind names for the three mesh-owned CRDs.
const (
	KindMeshPool      = "MeshPool"
	KindMeshRoute     = "MeshRoute"
	KindMeshTelemetry = "MeshTelemetry"
)

// Duration is a time span on the CRD wire: a Go time.ParseDuration string
// (e.g. "30s", "5m", "4h") — the same vocabulary the gateway's env-wired
// knobs (KSQUAD_MESH_LIFECYCLE_*) use today, now declarative. The CRD
// schema restricts it with the standard Kubernetes duration pattern.
type Duration string

// Parse converts the wire form to a time.Duration. Zero/empty fields are
// defaulted by the mechanism (see the *Defaults constants in
// meshpool_types.go), so callers usually parse only non-empty values.
func (d Duration) Parse() (time.Duration, error) {
	s := string(d)
	if s == "" {
		return 0, nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("v1alpha1: invalid duration %q: %w", s, err)
	}
	return v, nil
}

// TypeMeta mirrors the metav1.TypeMeta identity subset: apiVersion/kind on
// every top-level object. It is declared locally so the module stays
// dependency-free (see the package doc); the JSON tags match metav1 exactly.
type TypeMeta struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// ObjectMeta mirrors the metav1.ObjectMeta subset the mesh contracts need:
// namespaced identity, labels/annotations, and the generation/uid/resource
// bookkeeping a status read-back echoes. Declared locally for the same
// dependency-free reason as TypeMeta.
type ObjectMeta struct {
	Name              string            `json:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
}

// ListMeta mirrors the metav1.ListMeta subset for list results.
type ListMeta struct {
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Continue        string `json:"continue,omitempty"`
}
