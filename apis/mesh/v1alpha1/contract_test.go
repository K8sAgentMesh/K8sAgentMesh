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
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/K8sAgentMesh/K8sAgentMesh/mesh"
)

// fullMeshPool builds a MeshPool with every optional field set, so
// round-trip and deepcopy coverage cannot silently skip a field.
func fullMeshPool() *MeshPool {
	return &MeshPool{
		TypeMeta: TypeMeta{Kind: KindMeshPool, APIVersion: GroupVersion},
		ObjectMeta: ObjectMeta{
			Name:        "coder",
			Namespace:   "team-a",
			Labels:      map[string]string{"tier": "standing"},
			Annotations: map[string]string{"note": "test"},
		},
		Spec: MeshPoolSpec{
			PoolID: MeshPoolID{Team: "team-a", Role: "coder"},
			Envelope: MeshEnvelope{
				RuntimeType: "opencode",
				Model:       "qwen3-coder",
				ModelRoute: &MeshModelRoute{
					Endpoint: "http://ollama:11434/v1",
					Model:    "qwen3-coder",
					Token:    "",
				},
				Skills:   []string{"review", "tests"},
				Identity: &MeshAgentIdentity{Name: "a-coder", Squad: "alpha", Project: "mesh"},
				WorkDir:  "/workspace/team-a",
				Env:      map[string]string{"FOO": "bar"},
				Source: &MeshSourceSpec{
					TeamMainDir: "/workspace/team-a/main",
					WorktreeDir: "/workspace/team-a/wt/agent-coder-1",
					Branch:      "agent/coder/ISI-6086",
					BaseRef:     "main",
				},
			},
			Standing: StandingBand{Min: 2, Max: 4},
			Saturation: &SaturationPolicy{
				OverflowPolicy: OverflowPolicyGrow,
				GrowIdleShrink: "3m",
			},
			Lifecycle: &MeshLifecycleConfig{
				CheckInterval:   "30s",
				Hibernate:       true,
				IdleToHibernate: "5m",
				HibernateToStop: "4h",
				AlwaysUp:        true,
			},
			Registration: &RegistrationConfig{
				HeartbeatInterval: "30s",
				EndpointTTL:       "90s",
			},
		},
		Status: MeshPoolStatus{
			Standing:           2,
			StandingKnown:      true,
			Target:             2,
			Pod:                "gateway-team-a-0",
			ObservedGeneration: 7,
		},
	}
}

func TestMeshPoolJSONRoundTrip(t *testing.T) {
	in := fullMeshPool()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out MeshPool
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Round-trip via interface equality is brittle for TypeMeta embeds;
	// compare the spec/status payloads field by field via reflect on the
	// deep-copied shapes.
	if !reflect.DeepEqual(in.Spec, out.Spec) {
		t.Errorf("spec round-trip drifted:\n in  %+v\n out %+v", in.Spec, out.Spec)
	}
	if !reflect.DeepEqual(in.Status, out.Status) {
		t.Errorf("status round-trip drifted:\n in  %+v\n out %+v", in.Status, out.Status)
	}
	if out.Kind != KindMeshPool || out.APIVersion != GroupVersion {
		t.Errorf("typeMeta round-trip drifted: %+v", out.TypeMeta)
	}
}

func TestMeshPoolDeepCopyIndependence(t *testing.T) {
	in := fullMeshPool()
	cp := in.DeepCopy()
	cp.Spec.Envelope.Skills[0] = "mutated"
	cp.Spec.Envelope.Env["FOO"] = "mutated"
	cp.Spec.Envelope.Source.Branch = "mutated"
	cp.Spec.PoolID.Team = "mutated"
	cp.Status.Pod = "mutated"
	if in.Spec.Envelope.Skills[0] != "review" {
		t.Error("deepcopy aliasing: Skills slice shared")
	}
	if in.Spec.Envelope.Env["FOO"] != "bar" {
		t.Error("deepcopy aliasing: Env map shared")
	}
	if in.Spec.Envelope.Source.Branch != "agent/coder/ISI-6086" {
		t.Error("deepcopy aliasing: Source pointer shared")
	}
	if in.Spec.PoolID.Team != "team-a" {
		t.Error("deepcopy aliasing: PoolID shared")
	}
	if in.Status.Pod != "gateway-team-a-0" {
		t.Error("deepcopy aliasing: Status shared")
	}
}

func TestMeshRouteAndTelemetryRoundTrip(t *testing.T) {
	lru := false
	spans := false
	route := &MeshRoute{
		TypeMeta: TypeMeta{Kind: KindMeshRoute, APIVersion: GroupVersion},
		ObjectMeta: ObjectMeta{
			Name:      "cluster-default",
			Namespace: "mesh-system",
		},
		Spec: MeshRouteSpec{
			Match:       &MeshRouteMatch{Team: "team-a", Role: "coder"},
			Selection:   SelectionStrategyPreferIdle,
			LRUTiebreak: &lru,
			RecreateSLO: "3s",
			Fallback:    FallbackLegacy,
		},
		Status: MeshRouteStatus{RoutedTotal: 42, FallbackTotal: 2, P95ReadyMs: 1800},
	}
	telemetry := &MeshTelemetry{
		TypeMeta: TypeMeta{Kind: KindMeshTelemetry, APIVersion: GroupVersion},
		ObjectMeta: ObjectMeta{
			Name:      "default",
			Namespace: "mesh-system",
		},
		Spec: MeshTelemetrySpec{
			Scope:     &MeshTelemetryScope{Team: "team-a"},
			Emitter:   EmitterOTel,
			Namespace: "ksquad",
			Events:    []string{TelemetryEventRoute, TelemetryEventSpawn, TelemetryEventWake},
			OTel:      &OTelToggles{Spans: &spans, Metrics: nil},
		},
		Status: MeshTelemetryStatus{EmittedTotal: 100, LastEmitAt: "2026-10-17T11:00:00Z"},
	}
	for _, obj := range []any{route, telemetry} {
		b, err := json.Marshal(obj)
		if err != nil {
			t.Fatalf("marshal %T: %v", obj, err)
		}
		var back map[string]any
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal %T: %v", obj, err)
		}
		if _, ok := back["spec"].(map[string]any); !ok {
			t.Fatalf("%T: no spec in round-trip: %s", obj, b)
		}
	}
	if route.Spec.Selection != SelectionStrategyPreferIdle || *route.Spec.LRUTiebreak != false {
		t.Error("route spec drifted in construction")
	}
	if telemetry.Spec.Emitter != EmitterOTel || len(telemetry.Spec.Events) != 3 {
		t.Error("telemetry spec drifted in construction")
	}
	// Deepcopy independence for the pointer/optional surfaces.
	rc := route.DeepCopy()
	rc.Spec.Match.Team = "mutated"
	*rc.Spec.LRUTiebreak = true
	rc.Status.RoutedTotal = 0
	if route.Spec.Match.Team != "team-a" || *route.Spec.LRUTiebreak != false || route.Status.RoutedTotal != 42 {
		t.Error("MeshRoute deepcopy aliasing")
	}
	tc := telemetry.DeepCopy()
	tc.Spec.Scope.Team = "mutated"
	tc.Spec.Events[0] = "mutated"
	*tc.Spec.OTel.Spans = true
	tc.Status.EmittedTotal = 0
	if telemetry.Spec.Scope.Team != "team-a" || telemetry.Spec.Events[0] != TelemetryEventRoute ||
		*telemetry.Spec.OTel.Spans != false || telemetry.Status.EmittedTotal != 100 {
		t.Error("MeshTelemetry deepcopy aliasing")
	}
}

// TestDefaultsMirrorLiftedMechanism pins the CRD defaults to the lifted
// mechanism's constants (P3.5/P3.3): the contract documents what the code
// does, never a second opinion.
func TestDefaultsMirrorLiftedMechanism(t *testing.T) {
	for _, tc := range []struct {
		crddflt Duration
		mech    time.Duration
		what    string
	}{
		{DefaultCheckInterval, mesh.DefaultCheckInterval, "checkInterval"},
		{DefaultIdleToHibernate, mesh.DefaultIdleToHibernate, "idleToHibernate"},
		{DefaultHibernateToStop, mesh.DefaultHibernateToStop, "hibernateToStop"},
		{DefaultGrowIdleShrink, mesh.DefaultGrowIdleShrink, "growIdleShrink"},
	} {
		got, err := tc.crddflt.Parse()
		if err != nil {
			t.Errorf("%s: parse CRD default: %v", tc.what, err)
			continue
		}
		if got != tc.mech {
			t.Errorf("%s: CRD default %s != mechanism %s", tc.what, got, tc.mech)
		}
	}
	hb, err := DefaultHeartbeatInterval.Parse()
	if err != nil || hb != 30*time.Second {
		t.Errorf("heartbeatInterval default = %v (err %v), want 30s (the gateway register beat)", hb, err)
	}
	ttl, err := DefaultEndpointTTL.Parse()
	if err != nil || ttl != 90*time.Second {
		t.Errorf("endpointTTL default = %v (err %v), want 90s (three missed 30s beats)", ttl, err)
	}
	slo, err := DefaultRecreateSLO.Parse()
	if err != nil || slo != 3*time.Second {
		t.Errorf("recreateSLO default = %v (err %v), want 3s", slo, err)
	}
}

// TestTelemetryEventVocabularyMatchesP36 pins the events enum to the P3.6
// LifecycleObserver hook set: route|spawn|wake and deliberately NO
// hibernate entry (no ObserveHibernate hook exists — hibernation rides the
// process-state gauge).
func TestTelemetryEventVocabularyMatchesP36(t *testing.T) {
	events := []string{TelemetryEventRoute, TelemetryEventSpawn, TelemetryEventWake}
	for _, e := range events {
		if e != "route" && e != "spawn" && e != "wake" {
			t.Errorf("unexpected event vocabulary entry %q", e)
		}
	}
	for _, forbidden := range []string{"hibernate", "register", "deregister", "stop"} {
		for _, e := range events {
			if e == forbidden {
				t.Errorf("events vocabulary must not carry %q (no such LifecycleObserver hook in P3.6)", forbidden)
			}
		}
	}
}

// TestDurationWireForm guards the Duration wire vocabulary against the CRD
// pattern ('^-?([0-9]+(\.[0-9]+)?(ms|s|m|h))+$' — a leading minus is legal
// only where the mechanism gives it meaning, e.g. growIdleShrink < 0
// disables grown-slot shrinking).
func TestDurationWireForm(t *testing.T) {
	for _, ok := range []Duration{"30s", "5m", "4h", "2m", "3s", "1500ms", "1h30m", "0s", "-30s"} {
		if _, err := ok.Parse(); err != nil {
			t.Errorf("valid duration %q rejected: %v", ok, err)
		}
	}
	for _, bad := range []Duration{"1 w", "thirty", "30"} {
		if _, err := bad.Parse(); err == nil {
			t.Errorf("invalid duration %q accepted", bad)
		}
	}
	var zero Duration
	if d, err := zero.Parse(); err != nil || d != 0 {
		t.Errorf("empty duration must parse to 0, got %d err %v", d, err)
	}
}
