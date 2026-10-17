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

// SelectionStrategy is how the route leg picks a standing process
// (chapter §5.3). Today the mechanism is hardcoded prefer-idle + LRU; the
// CRD makes the mechanism declarative without moving the decision into host
// policy — selection preference is mesh mechanism, WHICH team routes is
// host policy (the rollout flag stays host-side, D9.3).
const (
	SelectionStrategyPreferIdle = "preferIdle"
	SelectionStrategyRandom     = "random"
	SelectionStrategySticky     = "sticky"
)

// FallbackPolicy is what the mesh leg does when it cannot route (no live
// registered endpoint, or a flagged registry error surfaced). Kept per the
// board decision on ISI-5827 interaction 826a7541: host #1 keeps the legacy
// spawn-sandbox leg as a permanent safety valve; host #2 has no legacy leg
// and uses "fail".
const (
	// FallbackLegacy: fall back to the host's pre-mesh dispatch leg — the
	// reversibility that keeps "flag on, mesh broken" from becoming
	// "flag on, runs dead" (empty-registry fallback, §5.2).
	FallbackLegacy = "legacy"
	// FallbackFail: no fallback — surface the error. Hosts without a
	// legacy leg.
	FallbackFail = "fail"
)

// DefaultRecreateSLO is the recreate-from-envelope target: A2A arrival →
// process-ready in ≤3s (the near-bare-exec path, chapter §6.1).
const DefaultRecreateSLO = Duration("3s")

// MeshRoute is the routing/selection policy object: how the mesh leg picks
// a standing process and what it does when it cannot. Mechanism only —
// per-team rollout flags, claim custody, and the at-most-once dispatch
// marker stay host-side (D3.3) and are deliberately NOT fields here.
type MeshRoute struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`
	Spec       MeshRouteSpec   `json:"spec,omitempty"`
	Status     MeshRouteStatus `json:"status,omitempty"`
}

// MeshRouteList is the list form of MeshRoute.
type MeshRouteList struct {
	TypeMeta `json:",inline"`
	ListMeta `json:"metadata,omitempty"`
	Items    []MeshRoute `json:"items"`
}

// MeshRouteMatch scopes one MeshRoute: absent means the cluster default.
type MeshRouteMatch struct {
	// Team scopes the route policy to one team's dispatches.
	Team string `json:"team,omitempty"`
	// Role scopes the route policy to one role. Empty matches every role
	// of the team.
	Role string `json:"role,omitempty"`
}

// MeshRouteSpec is the desired routing/selection behavior.
type MeshRouteSpec struct {
	// Match scopes the policy; absent = cluster default.
	Match *MeshRouteMatch `json:"match,omitempty"`
	// Selection is the standing-process pick strategy. Empty behaves as
	// "preferIdle" (today's mechanism).
	// +kubebuilder:validation:Enum=preferIdle;random;sticky
	Selection string `json:"selection,omitempty"`
	// LRUTiebreak orders equal-preference candidates least-recently-seen
	// first (§5.3 LRU tie-break: spread wear, keep caches warm). nil
	// behaves as true.
	LRUTiebreak *bool `json:"lruTiebreak,omitempty"`
	// RecreateSLO is the status-facing target the route leg observes:
	// arrival → process-ready on the overflow/recreate path. Empty takes
	// DefaultRecreateSLO (3s).
	RecreateSLO Duration `json:"recreateSlo,omitempty"`
	// Fallback is the no-route behavior. Empty behaves as "legacy"
	// (host #1); hosts without a legacy leg set "fail".
	// +kubebuilder:validation:Enum=legacy;fail
	Fallback string `json:"fallback,omitempty"`
}

// MeshRouteStatus is the bounded route telemetry read-back.
type MeshRouteStatus struct {
	// RoutedTotal is the count of mesh-leg routed dispatches.
	RoutedTotal int64 `json:"routedTotal,omitempty"`
	// FallbackTotal is the count of dispatches that took the fallback.
	FallbackTotal int64 `json:"fallbackTotal,omitempty"`
	// P95ReadyMs is the p95 arrival → process-ready latency, milliseconds.
	P95ReadyMs int64 `json:"p95ReadyMs,omitempty"`
}
