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

// Benchmarks for the mesh route leg (ISI-6252). These capture baselines for
// the three pure, allocation-sensitive functions on the dispatch path, so
// CI job 9 (performance) graduates from informational PR-comment reporting
// to a benchmark-action comparison gate (ISI-6106 Phase 2). They reuse the
// fakeLookup seam from routeselect_test.go — no live registry, no I/O — so
// the numbers reflect decision/parsing cost alone, not network latency.

// BenchmarkRoute measures the per-dispatch route decision: flag check,
// registry lookup via the fake, and the /task URL assembly (the mesh-leg
// hot path taken for every flagged-team dispatch).
func BenchmarkRoute(b *testing.B) {
	ctx := context.Background()
	cfg := &RouteConfig{
		Teams:  TeamSet{"team-acme": {}},
		Lookup: fakeLookup{eps: []Endpoint{{EndpointID: "e0", Endpoint: "http://10.0.0.1:8081"}}},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, use, err := Route(ctx, cfg, "team-acme", "coder"); !use || err != nil {
			b.Fatalf("use=%v err=%v, want mesh route", use, err)
		}
	}
}

// BenchmarkIsSaturation measures the 409-refusal classification on the
// re-drive path — a string match run on every mesh-leg submit error to
// decide whether the host retries (saturation) or surfaces the failure.
func BenchmarkIsSaturation(b *testing.B) {
	err := fmt.Errorf("a2a: POST http://10.0.0.1:4090/task: supervisor answered 409 Conflict: %v", ErrProcessBusy)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !IsSaturation(err) {
			b.Fatal("409 + busy sentinel must read as saturation")
		}
	}
}

// BenchmarkNewTeamSet measures canary-list parsing (the per-team rollout
// flag), run at config load and on every config reload.
func BenchmarkNewTeamSet(b *testing.B) {
	const csv = " team-acme , team-beta , team-gamma , , team-delta "
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if s := NewTeamSet(csv); len(s) != 4 {
			b.Fatalf("parsed %d teams, want 4", len(s))
		}
	}
}
