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
	"sync"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/mesh"
)

// Regrounding is the working context handed back to a resumed actor: the facts
// the resume-reground read out of durable memory for the actor's scope. In the
// walking skeleton the surcouche delivers it to a RegroundSink; a production
// surcouche would inject it into the woken runtime's prompt/context window.
type Regrounding struct {
	Scope Scope
	Facts []Fact
}

// RegroundSink receives a Regrounding when an actor resumes. It is the seam
// where "reload the context" becomes concrete; the skeleton's test uses it to
// assert the regrounded view is consistent with what was flushed. It MUST NOT
// block the Wake path meaningfully — reground is on the instant-wake critical
// path the substrate advertises.
type RegroundSink interface {
	OnReground(ctx context.Context, r Regrounding)
}

// RegroundFunc adapts a plain function to a RegroundSink.
type RegroundFunc func(ctx context.Context, r Regrounding)

// OnReground calls f.
func (f RegroundFunc) OnReground(ctx context.Context, r Regrounding) { f(ctx, r) }

// Surcouche is the AgentMesh identity+memory overlay. It decorates a substrate
// mesh.RuntimeAdapter: it binds each Handle to a memory Scope at Spawn,
// interposes suspend-flush on Hibernate and resume-reground on Wake, and passes
// every other lifecycle verb straight through. The substrate stays unaware that
// memory exists — the layering the repositioning synthesis calls for.
//
// It itself satisfies mesh.RuntimeAdapter, so it drops in wherever the gateway
// expects an adapter: the gateway drives existence/reachability as before and
// the surcouche silently owns the memory boundary around suspend/resume.
type Surcouche struct {
	substrate mesh.RuntimeAdapter
	memory    MemoryContract
	reground  RegroundSink

	mu       sync.Mutex
	bindings map[string]Scope // handle ID -> memory scope
}

// Options configures a Surcouche.
type Options struct {
	// Reground, when set, receives a Regrounding on every Wake. Optional: a nil
	// sink means reground still reads durable memory (proving the read works)
	// but has nowhere to deliver it.
	Reground RegroundSink
}

// New wraps substrate with the AgentMesh surcouche backed by mem. It panics if
// either dependency is nil — a surcouche with no substrate or no memory is a
// programming error, not a runtime condition.
func New(substrate mesh.RuntimeAdapter, mem MemoryContract, opts Options) *Surcouche {
	if substrate == nil {
		panic("agentmesh: nil substrate adapter")
	}
	if mem == nil {
		panic("agentmesh: nil memory contract")
	}
	return &Surcouche{
		substrate: substrate,
		memory:    mem,
		reground:  opts.Reground,
		bindings:  make(map[string]Scope),
	}
}

var _ mesh.RuntimeAdapter = (*Surcouche)(nil)

// ScopeFromIdentity derives the memory scope of a standing actor from its
// Spawn-envelope identity. It is the one place identity → memory binding is
// defined, so a host that keys memory differently overrides exactly here.
func ScopeFromIdentity(id a2a.AgentIdentity) Scope {
	return Scope{Agent: id.Name, Squad: id.Squad, Project: id.Project}
}

// Spawn delegates to the substrate, then records the handle → scope binding so
// a later Hibernate/Wake on the same handle knows whose memory to flush and
// reground. A spawn that fails records nothing.
func (s *Surcouche) Spawn(ctx context.Context, env mesh.Envelope) (mesh.Handle, a2a.AgentCard, error) {
	h, card, err := s.substrate.Spawn(ctx, env)
	if err != nil {
		return h, card, err
	}
	s.bind(h.ID(), ScopeFromIdentity(env.Identity))
	return h, card, nil
}

// Hibernate is the suspend-flush handler: it FIRST flushes the actor's working
// memory to durable storage — a distinct AgentMesh operation, outside anything
// the substrate snapshots — and only THEN lets the substrate freeze the
// process. Order matters: freeze-first could strand un-flushed working facts in
// a stopped process. A flush error aborts the hibernate so the caller never
// believes an actor was safely suspended while its memory was lost.
func (s *Surcouche) Hibernate(ctx context.Context, h mesh.Handle) error {
	scope, ok := s.scopeOf(h.ID())
	if ok {
		if _, err := s.memory.Flush(ctx, scope); err != nil {
			return fmt.Errorf("agentmesh: suspend-flush for %s failed, hibernate aborted: %w", h.ID(), err)
		}
	}
	return s.substrate.Hibernate(ctx, h)
}

// Wake is the resume-reground handler: it FIRST lets the substrate restore the
// process (SIGCONT, no re-init), THEN regrounds by re-reading durable DQL for
// the actor's scope and delivering it to the RegroundSink. A substrate wake
// failure short-circuits reground — there is no live actor to reground. A
// reground read failure is returned so the caller can retry the reground
// without re-waking.
func (s *Surcouche) Wake(ctx context.Context, h mesh.Handle) error {
	if err := s.substrate.Wake(ctx, h); err != nil {
		return err
	}
	scope, ok := s.scopeOf(h.ID())
	if !ok {
		return nil
	}
	facts, err := s.memory.Query(ctx, Query{Scope: scope})
	if err != nil {
		return fmt.Errorf("agentmesh: resume-reground for %s failed: %w", h.ID(), err)
	}
	if s.reground != nil {
		s.reground.OnReground(ctx, Regrounding{Scope: scope, Facts: facts})
	}
	return nil
}

// Stop terminates the process via the substrate and drops the handle's binding.
// Memory is NOT purged: durable knowledge outlives the process — a recreated or
// successor actor in the same scope regrounds from it.
func (s *Surcouche) Stop(ctx context.Context, h mesh.Handle) error {
	err := s.substrate.Stop(ctx, h)
	s.unbind(h.ID())
	return err
}

// Recreate rebuilds via the substrate and rebinds the NEW handle to the
// envelope's scope, retiring the old handle's binding. The prior handle is
// invalid after a successful Recreate, exactly as the substrate contract says.
func (s *Surcouche) Recreate(ctx context.Context, h mesh.Handle, env mesh.Envelope) (mesh.Handle, error) {
	nh, err := s.substrate.Recreate(ctx, h, env)
	if err != nil {
		return nh, err
	}
	s.unbind(h.ID())
	s.bind(nh.ID(), ScopeFromIdentity(env.Identity))
	return nh, nil
}

// Register passes through — card announcement is reachability, not memory.
func (s *Surcouche) Register(ctx context.Context, h mesh.Handle) (a2a.AgentCard, error) {
	return s.substrate.Register(ctx, h)
}

// Route passes through — task delivery is the gateway/reachability authority.
// A cycle's memory Writes happen through the MemoryContract during the task,
// not on this path.
func (s *Surcouche) Route(ctx context.Context, h mesh.Handle, task a2a.Task) (mesh.Stream, error) {
	return s.substrate.Route(ctx, h, task)
}

func (s *Surcouche) bind(id string, scope Scope) {
	s.mu.Lock()
	s.bindings[id] = scope
	s.mu.Unlock()
}

func (s *Surcouche) unbind(id string) {
	s.mu.Lock()
	delete(s.bindings, id)
	s.mu.Unlock()
}

func (s *Surcouche) scopeOf(id string) (Scope, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, ok := s.bindings[id]
	return scope, ok
}
