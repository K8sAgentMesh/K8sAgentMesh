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

package agentmesh

import (
	"context"
	"errors"
	"testing"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/mesh"
)

// fakeHandle is a minimal mesh.Handle for the skeleton test.
type fakeHandle struct{ id, ep string }

func (h fakeHandle) ID() string       { return h.id }
func (h fakeHandle) Endpoint() string { return h.ep }

// fakeSubstrate is a mesh.RuntimeAdapter standing in for the existence layer.
// It records the order of Hibernate/Wake calls so the test can assert the
// surcouche sequences flush-before-freeze and wake-before-reground, and lets a
// test inject a Wake error to exercise the short-circuit.
type fakeSubstrate struct {
	calls    []string
	wakeErr  error
	spawnErr error
	nextID   int
}

func (f *fakeSubstrate) Spawn(_ context.Context, env mesh.Envelope) (mesh.Handle, a2a.AgentCard, error) {
	if f.spawnErr != nil {
		return nil, a2a.AgentCard{}, f.spawnErr
	}
	f.nextID++
	f.calls = append(f.calls, "spawn")
	return fakeHandle{id: env.Identity.Name, ep: "http://127.0.0.1:0"}, a2a.AgentCard{}, nil
}

func (f *fakeSubstrate) Register(context.Context, mesh.Handle) (a2a.AgentCard, error) {
	return a2a.AgentCard{}, nil
}

func (f *fakeSubstrate) Route(context.Context, mesh.Handle, a2a.Task) (mesh.Stream, error) {
	return nil, nil
}

func (f *fakeSubstrate) Hibernate(context.Context, mesh.Handle) error {
	f.calls = append(f.calls, "hibernate")
	return nil
}

func (f *fakeSubstrate) Wake(context.Context, mesh.Handle) error {
	f.calls = append(f.calls, "wake")
	return f.wakeErr
}

func (f *fakeSubstrate) Stop(context.Context, mesh.Handle) error {
	f.calls = append(f.calls, "stop")
	return nil
}

func (f *fakeSubstrate) Recreate(_ context.Context, _ mesh.Handle, env mesh.Envelope) (mesh.Handle, error) {
	f.nextID++
	f.calls = append(f.calls, "recreate")
	return fakeHandle{id: env.Identity.Name + "-r", ep: "http://127.0.0.1:0"}, nil
}

var researcher = a2a.AgentIdentity{Name: "researcher", Squad: "mesh", Project: "isi-7417"}

func spawnEnv() mesh.Envelope {
	return mesh.Envelope{RuntimeType: "opencode", Identity: researcher}
}

// TestWalkingSkeleton_CycleSuspendResume is the minimal e2e the issue asks for:
// an agent cycle writes knowledge, the actor suspends (flush), then resumes
// (reground), and the regrounded memory is consistent with what was written.
func TestWalkingSkeleton_CycleSuspendResume(t *testing.T) {
	ctx := context.Background()
	store := NewSkeletonStore()
	var got Regrounding
	sub := &fakeSubstrate{}
	sc := New(sub, store, Options{
		Reground: RegroundFunc(func(_ context.Context, r Regrounding) { got = r }),
	})

	// Spawn binds the actor's identity to its memory scope.
	h, _, err := sc.Spawn(ctx, spawnEnv())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	scope := ScopeFromIdentity(researcher)

	// Active agent cycle writes shared knowledge through the memory contract.
	if _, err := store.Write(ctx, WriteRequest{
		Scope:   scope,
		CycleID: "cycle-1",
		Facts: []FactInput{
			{Subject: "cluster-x", Key: "k8s-version", Value: "1.31"},
			{Subject: "cluster-x", Key: "cni", Value: "cilium"},
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Pre-suspend the writes are working-only: nothing durable yet.
	if n := store.WorkingLen(scope); n != 2 {
		t.Fatalf("working len before flush = %d, want 2", n)
	}
	if facts, _ := store.Query(ctx, Query{Scope: scope}); len(facts) != 0 {
		t.Fatalf("durable facts before flush = %d, want 0", len(facts))
	}

	// Suspend: the surcouche must flush BEFORE the substrate freezes.
	if err := sc.Hibernate(ctx, h); err != nil {
		t.Fatalf("hibernate: %v", err)
	}
	if store.WorkingLen(scope) != 0 {
		t.Fatalf("working set not drained by suspend-flush")
	}
	assertOrder(t, sub.calls, "spawn", "hibernate")

	// Resume: the surcouche wakes the substrate THEN regrounds from durable DQL.
	if err := sc.Wake(ctx, h); err != nil {
		t.Fatalf("wake: %v", err)
	}
	assertOrder(t, sub.calls, "spawn", "hibernate", "wake")

	// The regrounded view is consistent with what the cycle wrote and flushed.
	if len(got.Facts) != 2 {
		t.Fatalf("regrounded facts = %d, want 2", len(got.Facts))
	}
	if v := factValue(got.Facts, "cluster-x", "k8s-version"); v != "1.31" {
		t.Fatalf("regrounded k8s-version = %q, want 1.31", v)
	}
	if v := factValue(got.Facts, "cluster-x", "cni"); v != "cilium" {
		t.Fatalf("regrounded cni = %q, want cilium", v)
	}
}

// TestSuspendWithoutFlushLosesWorkingSet pins the load-bearing claim: the
// substrate snapshot does NOT carry memory, so writes that are never flushed do
// not survive a suspend. Here the substrate is driven directly (no surcouche),
// modeling a suspend that skips the AgentMesh flush — the working set is lost
// and a reground query comes back empty.
func TestSuspendWithoutFlushLosesWorkingSet(t *testing.T) {
	ctx := context.Background()
	store := NewSkeletonStore()
	scope := ScopeFromIdentity(researcher)

	if _, err := store.Write(ctx, WriteRequest{
		Scope: scope, CycleID: "cycle-1",
		Facts: []FactInput{{Subject: "cluster-x", Key: "k8s-version", Value: "1.31"}},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Substrate suspends/resumes WITHOUT an AgentMesh flush. Nothing durable.
	sub := &fakeSubstrate{}
	if err := sub.Hibernate(ctx, fakeHandle{id: "researcher"}); err != nil {
		t.Fatalf("hibernate: %v", err)
	}
	if err := sub.Wake(ctx, fakeHandle{id: "researcher"}); err != nil {
		t.Fatalf("wake: %v", err)
	}

	facts, _ := store.Query(ctx, Query{Scope: scope})
	if len(facts) != 0 {
		t.Fatalf("un-flushed working set survived suspend: got %d durable facts", len(facts))
	}
}

// TestFlushAbortsHibernateOnError proves the surcouche never reports a clean
// suspend when the flush failed — an actor must not freeze believing its memory
// was saved.
func TestFlushAbortsHibernateOnError(t *testing.T) {
	ctx := context.Background()
	sub := &fakeSubstrate{}
	sc := New(sub, failingFlushStore{}, Options{})
	h, _, err := sc.Spawn(ctx, spawnEnv())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := sc.Hibernate(ctx, h); err == nil {
		t.Fatalf("hibernate succeeded despite flush failure")
	}
	for _, c := range sub.calls {
		if c == "hibernate" {
			t.Fatalf("substrate was frozen after a failed flush")
		}
	}
}

// TestWakeErrorShortCircuitsReground proves reground never runs against a
// substrate that failed to wake.
func TestWakeErrorShortCircuitsReground(t *testing.T) {
	ctx := context.Background()
	sub := &fakeSubstrate{wakeErr: errors.New("sigcont failed")}
	regrounded := false
	sc := New(sub, NewSkeletonStore(), Options{
		Reground: RegroundFunc(func(context.Context, Regrounding) { regrounded = true }),
	})
	h, _, _ := sc.Spawn(ctx, spawnEnv())
	if err := sc.Wake(ctx, h); err == nil {
		t.Fatalf("wake error not surfaced")
	}
	if regrounded {
		t.Fatalf("regrounded after a failed wake")
	}
}

// TestRecreateRebindsScope proves durable memory outlives a Recreate and the
// successor handle regrounds from it.
func TestRecreateRebindsScope(t *testing.T) {
	ctx := context.Background()
	store := NewSkeletonStore()
	scope := ScopeFromIdentity(researcher)
	_, _ = store.Write(ctx, WriteRequest{Scope: scope, CycleID: "c1",
		Facts: []FactInput{{Subject: "s", Key: "k", Value: "v"}}})
	_, _ = store.Flush(ctx, scope)

	var got Regrounding
	sc := New(&fakeSubstrate{}, store, Options{
		Reground: RegroundFunc(func(_ context.Context, r Regrounding) { got = r }),
	})
	h, _, _ := sc.Spawn(ctx, spawnEnv())
	nh, err := sc.Recreate(ctx, h, spawnEnv())
	if err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if err := sc.Wake(ctx, nh); err != nil {
		t.Fatalf("wake after recreate: %v", err)
	}
	if len(got.Facts) != 1 || got.Facts[0].Value != "v" {
		t.Fatalf("successor did not reground durable memory: %+v", got.Facts)
	}
}

// failingFlushStore is a MemoryContract whose Flush always errors.
type failingFlushStore struct{}

func (failingFlushStore) Write(context.Context, WriteRequest) (int, error) { return 0, nil }
func (failingFlushStore) Flush(context.Context, Scope) (int, error) {
	return 0, errors.New("dql unavailable")
}
func (failingFlushStore) Query(context.Context, Query) ([]Fact, error) { return nil, nil }

func assertOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call order = %v, want %v", got, want)
		}
	}
}

func factValue(facts []Fact, subject, key string) string {
	for _, f := range facts {
		if f.Subject == subject && f.Key == key {
			return f.Value
		}
	}
	return ""
}
