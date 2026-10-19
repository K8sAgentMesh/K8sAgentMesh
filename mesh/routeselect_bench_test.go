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

package mesh

import (
	"context"
	"fmt"
	"testing"
)

// Benchmarks for the route-selection hot path (ISI-6252, unblocks the CI
// perf gate for ISI-6106). Route runs on every dispatch and IsSaturation on
// every mesh-leg submit error, so these are the per-dispatch costs a
// regression baseline actually wants to watch. They reuse the test fixtures
// (fakeLookup, Endpoint) so the benchmark measures the decision logic, not
// any registry I/O — the Lookup seam is bound to an in-memory fake.
//
// Package-level sinks defeat dead-code elimination: without a consumed
// result the optimizer may delete the call under test.
var (
	benchURL string
	benchUse bool
	benchSat bool
	benchSet TeamSet
)

// BenchmarkRoute measures both live branches of the per-dispatch decision:
// the mesh-route pick (flagged team, live endpoint) and the legacy no-op
// (flag off cluster-wide). The two dominate every dispatch's routing cost.
func BenchmarkRoute(b *testing.B) {
	ctx := context.Background()
	team, role := "team-acme", "coder"
	eps := []Endpoint{{EndpointID: "e0", Endpoint: "http://10.0.0.1:8081"}}

	b.Run("mesh_route", func(b *testing.B) {
		cfg := &RouteConfig{Teams: TeamSet{team: {}}, Lookup: fakeLookup{eps: eps}}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchURL, benchUse, _ = Route(ctx, cfg, team, role)
		}
	})

	b.Run("legacy_noop", func(b *testing.B) {
		cfg := &RouteConfig{Teams: TeamSet{"other": {}}, Lookup: fakeLookup{eps: eps}}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchURL, benchUse, _ = Route(ctx, cfg, team, role)
		}
	})
}

// BenchmarkIsSaturation measures the saturation-match cost paid on every
// mesh-leg submit error — a string scan over the folded status + sentinel.
func BenchmarkIsSaturation(b *testing.B) {
	err := fmt.Errorf("a2a: POST http://10.0.0.1:4090/task: supervisor answered 409 Conflict: %v", ErrProcessBusy)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSat = IsSaturation(err)
	}
}

// BenchmarkNewTeamSet measures canary-list parsing (config load, D9.3).
func BenchmarkNewTeamSet(b *testing.B) {
	const csv = "team-acme, team-bravo , team-charlie,team-delta , team-echo"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchSet = NewTeamSet(csv)
	}
}
