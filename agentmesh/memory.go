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
	"fmt"
	"sort"
	"sync"
)

// Scope is the identity binding of a slice of AgentMesh memory: whose knowledge
// it is (the standing actor) and which project it belongs to. It mirrors the
// identity axes of a2a.AgentIdentity (Name/Squad/Project) that ride the Spawn
// envelope, so the surcouche can derive a Scope from a substrate handle without
// the substrate itself carrying any memory concept.
type Scope struct {
	// Agent is the standing actor identity (a2a.AgentIdentity.Name).
	Agent string
	// Squad groups actors; part of the key so two squads' memories never alias.
	Squad string
	// Project is the memory partition the knowledge belongs to.
	Project string
}

// key is the stable map key for a scope — the DQL partition selector.
func (s Scope) key() string { return s.Agent + "\x1f" + s.Squad + "\x1f" + s.Project }

// Fact is one unit of shared knowledge in AgentMesh memory, modeled as a
// DQL/Grail triple within a scope: Subject (the entity the fact is about), Key
// (the attribute/predicate) and Value (the object). CycleID records which agent
// cycle wrote it, and Seq is a monotonic write order within the store so a
// reground reads facts in a deterministic, reproducible order.
type Fact struct {
	Scope   Scope
	Subject string
	Key     string
	Value   string
	CycleID string
	Seq     uint64
}

// FactInput is one fact a cycle wants to write — a Fact without the
// store-assigned Scope/CycleID/Seq, which Write stamps on.
type FactInput struct {
	Subject string
	Key     string
	Value   string
}

// Query is a minimal DQL/Grail read over one scope: fetch the scope's facts,
// optionally narrowed to a Subject and/or Key. An empty Subject or Key matches
// any. This is deliberately the smallest selector that proves the seam — a real
// DQL/Grail query language is out of scope for the walking skeleton.
type Query struct {
	Scope   Scope
	Subject string // empty = any
	Key     string // empty = any
}

// WriteRequest is a batch of facts written from a single active agent cycle.
// Writes land in the scope's volatile working set; they become durable (and so
// survivable across a substrate suspend) only once the scope is flushed — the
// semantic the suspend-flush handler exists to enforce.
type WriteRequest struct {
	Scope   Scope
	CycleID string
	Facts   []FactInput
}

// MemoryContract is the AgentMesh DQL/Grail seam — the identity+memory
// authority of the surcouche. It is intentionally distinct from the substrate's
// snapshot: the substrate persists process state, this persists knowledge.
//
//   - Write records facts from an active cycle into the scope's WORKING set.
//   - Flush promotes a scope's working set into DURABLE storage — the
//     suspend-flush barrier. It is a distinct AgentMesh operation precisely
//     because the substrate snapshot does not carry memory.
//   - Query reads DURABLE facts back — the reground read on resume, and the
//     steady-state "query shared knowledge" path.
type MemoryContract interface {
	// Write stages facts from cycleID into scope's working set and returns the
	// number staged.
	Write(ctx context.Context, w WriteRequest) (int, error)
	// Flush promotes scope's working set into durable storage and returns the
	// number of facts made durable. It is idempotent: a flush with an empty
	// working set is a no-op that returns 0.
	Flush(ctx context.Context, scope Scope) (int, error)
	// Query returns the durable facts matching q, in write (Seq) order.
	Query(ctx context.Context, q Query) ([]Fact, error)
}

// SkeletonStore is the in-memory, two-tier MemoryContract the walking skeleton
// runs against. The two tiers ARE the proof: a cycle's Write lands in the
// volatile working set, and only Flush moves it to the durable tier. A suspend
// that skips flush loses the working set — the same way a real substrate whose
// snapshot has no memory would — while a flushed scope regrounds cleanly on
// resume. A production surcouche swaps this for a DQL/Grail-backed store with
// the same contract.
type SkeletonStore struct {
	mu      sync.Mutex
	seq     uint64
	working map[string][]Fact // scope key -> staged, not-yet-durable facts
	durable map[string][]Fact // scope key -> flushed, survivable facts
}

// NewSkeletonStore returns an empty two-tier store.
func NewSkeletonStore() *SkeletonStore {
	return &SkeletonStore{
		working: make(map[string][]Fact),
		durable: make(map[string][]Fact),
	}
}

// Write stages w.Facts in the scope's working set.
func (s *SkeletonStore) Write(_ context.Context, w WriteRequest) (int, error) {
	if w.CycleID == "" {
		return 0, fmt.Errorf("agentmesh: write without a cycle id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := w.Scope.key()
	for _, in := range w.Facts {
		if in.Key == "" {
			return 0, fmt.Errorf("agentmesh: fact for subject %q has empty key", in.Subject)
		}
		s.seq++
		s.working[k] = append(s.working[k], Fact{
			Scope:   w.Scope,
			Subject: in.Subject,
			Key:     in.Key,
			Value:   in.Value,
			CycleID: w.CycleID,
			Seq:     s.seq,
		})
	}
	return len(w.Facts), nil
}

// Flush promotes the scope's working set into the durable tier. Later writes to
// the same (subject,key) supersede earlier durable facts, so a flushed scope
// holds one current value per attribute — the reground sees the latest truth.
func (s *SkeletonStore) Flush(_ context.Context, scope Scope) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := scope.key()
	staged := s.working[k]
	if len(staged) == 0 {
		return 0, nil
	}
	durable := s.durable[k]
	for _, f := range staged {
		durable = upsert(durable, f)
	}
	sort.SliceStable(durable, func(i, j int) bool { return durable[i].Seq < durable[j].Seq })
	s.durable[k] = durable
	delete(s.working, k)
	return len(staged), nil
}

// Query returns durable facts in the scope matching q's Subject/Key filters, in
// Seq order.
func (s *SkeletonStore) Query(_ context.Context, q Query) ([]Fact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Fact
	for _, f := range s.durable[q.Scope.key()] {
		if q.Subject != "" && f.Subject != q.Subject {
			continue
		}
		if q.Key != "" && f.Key != q.Key {
			continue
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// WorkingLen reports how many facts are staged-but-not-durable for a scope. It
// exists for the e2e test to assert the flush boundary (working drains to zero
// after flush) and for a surcouche health probe.
func (s *SkeletonStore) WorkingLen(scope Scope) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.working[scope.key()])
}

// upsert replaces a same-(subject,key) fact in place (keeping the newer value
// and its Seq) or appends it when the attribute is new.
func upsert(facts []Fact, f Fact) []Fact {
	for i := range facts {
		if facts[i].Subject == f.Subject && facts[i].Key == f.Key {
			facts[i] = f
			return facts
		}
	}
	return append(facts, f)
}
