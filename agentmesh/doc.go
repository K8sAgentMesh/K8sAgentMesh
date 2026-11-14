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

// Package agentmesh is the AgentMesh surcouche: the identity+memory layer that
// composes OVER an existence substrate and a reachability layer, never inside
// them. It is the walking skeleton validating the repositioning synthesis
// (design plan on the planning issue) on top of the agent substrate +
// agentgateway split:
//
//   - substrate  = the EXISTENCE layer: actors, snapshot, suspend/resume.
//     In this repo the mesh package's RuntimeAdapter (Hibernate/Wake =
//     SIGSTOP/SIGCONT) stands in for the substrate's suspend/resume primitive.
//   - agentgateway = the REACHABILITY layer: A2A/MCP routing and discovery.
//     The surcouche does not touch it — routing stays the gateway's authority.
//   - agentmesh  = the IDENTITY+MEMORY layer this package builds.
//
// Three layers, three authorities. The load-bearing claim the skeleton proves
// is the memory boundary: AgentMesh memory is a DISTINCT authority from the
// substrate snapshot. When the substrate suspends an actor it freezes process
// state; it does NOT persist the actor's working knowledge. So the surcouche
// runs its own suspend-flush (promote the cycle's working set to durable DQL
// storage) BEFORE the substrate freezes, and its own resume-reground (re-read
// DQL to reload the working context) AFTER the substrate restores. Skip the
// flush and the working set dies with the freeze exactly as it would on a real
// substrate whose snapshot has no memory — which is why flush is a first-class
// AgentMesh operation and not a substrate concern.
//
// Surcouche (surcouche.go) is a decorator over mesh.RuntimeAdapter: it binds
// each substrate Handle to a memory Scope at Spawn, interposes flush on
// Hibernate and reground on Wake, and passes every other lifecycle verb through
// untouched — the substrate never learns that memory exists. MemoryContract
// (memory.go) is the DQL/Grail seam (query shared knowledge + write from an
// active cycle + flush); SkeletonStore is the in-memory two-tier
// (working/durable) implementation the prototype and its e2e test run against.
package agentmesh
