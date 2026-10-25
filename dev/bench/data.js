window.BENCHMARK_DATA = {
  "lastUpdate": 1792948444754,
  "repoUrl": "https://github.com/K8sAgentMesh/K8sAgentMesh",
  "entries": {
    "Benchmark": [
      {
        "commit": {
          "author": {
            "email": "81800915+henrikrexed@users.noreply.github.com",
            "name": "Henrik Rexed",
            "username": "henrikrexed"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "a3aa7e6ccfd4031028a78cd2e38ea8dda4c74519",
          "message": "Merge pull request #32 from K8sAgentMesh/feat/isi-6106-perf-comparison-gate\n\nci(perf): graduate benchmark job to a comparison gate (ISI-6106)",
          "timestamp": "2026-10-19T14:22:24+01:00",
          "tree_id": "30f897d98d584a1d326a951da4bfd435a62106a4",
          "url": "https://github.com/K8sAgentMesh/K8sAgentMesh/commit/a3aa7e6ccfd4031028a78cd2e38ea8dda4c74519"
        },
        "date": 1792416587901,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkRoute",
            "value": 44.77,
            "unit": "ns/op\t      32 B/op\t       1 allocs/op",
            "extra": "29507342 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - ns/op",
            "value": 44.77,
            "unit": "ns/op",
            "extra": "29507342 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - B/op",
            "value": 32,
            "unit": "B/op",
            "extra": "29507342 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - allocs/op",
            "value": 1,
            "unit": "allocs/op",
            "extra": "29507342 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation",
            "value": 26.93,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "44080789 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - ns/op",
            "value": 26.93,
            "unit": "ns/op",
            "extra": "44080789 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "44080789 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "44080789 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet",
            "value": 258.9,
            "unit": "ns/op\t     336 B/op\t       3 allocs/op",
            "extra": "4652708 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - ns/op",
            "value": 258.9,
            "unit": "ns/op",
            "extra": "4652708 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - B/op",
            "value": 336,
            "unit": "B/op",
            "extra": "4652708 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - allocs/op",
            "value": 3,
            "unit": "allocs/op",
            "extra": "4652708 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "81800915+henrikrexed@users.noreply.github.com",
            "name": "Henrik Rexed",
            "username": "henrikrexed"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "7d23e549ed249d39e42ddeb82241f8e16ab6ef17",
          "message": "Merge pull request #33 from K8sAgentMesh/mesh/isi-6130-claudecode-adapter\n\nfeat(mesh+agentruntime): claude-code engine + NewClaudeCodeAdapter (ISI-6130 P4.4)",
          "timestamp": "2026-10-23T22:46:25+02:00",
          "tree_id": "6c6f887f5334580db14d5648a4081f8bd9efe30a",
          "url": "https://github.com/K8sAgentMesh/K8sAgentMesh/commit/7d23e549ed249d39e42ddeb82241f8e16ab6ef17"
        },
        "date": 1792788419927,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkRoute",
            "value": 49.83,
            "unit": "ns/op\t      32 B/op\t       1 allocs/op",
            "extra": "23774275 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - ns/op",
            "value": 49.83,
            "unit": "ns/op",
            "extra": "23774275 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - B/op",
            "value": 32,
            "unit": "B/op",
            "extra": "23774275 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - allocs/op",
            "value": 1,
            "unit": "allocs/op",
            "extra": "23774275 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation",
            "value": 37.52,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "32899207 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - ns/op",
            "value": 37.52,
            "unit": "ns/op",
            "extra": "32899207 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "32899207 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "32899207 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet",
            "value": 244.8,
            "unit": "ns/op\t     336 B/op\t       3 allocs/op",
            "extra": "4884282 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - ns/op",
            "value": 244.8,
            "unit": "ns/op",
            "extra": "4884282 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - B/op",
            "value": 336,
            "unit": "B/op",
            "extra": "4884282 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - allocs/op",
            "value": 3,
            "unit": "allocs/op",
            "extra": "4884282 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "81800915+henrikrexed@users.noreply.github.com",
            "name": "Henrik Rexed",
            "username": "henrikrexed"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "22910c5a75913a521662400012d9f857de8e3e79",
          "message": "Merge pull request #34 from K8sAgentMesh/mesh/isi-6547-crewai-pr\n\nfeat(mesh): CrewAI integration P4.3 — adapter #5 + crewaiRouter wire translation",
          "timestamp": "2026-10-24T11:41:33+02:00",
          "tree_id": "1996499b670b4add8cd9c22c845d562597a1dcb7",
          "url": "https://github.com/K8sAgentMesh/K8sAgentMesh/commit/22910c5a75913a521662400012d9f857de8e3e79"
        },
        "date": 1792834924676,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkRoute",
            "value": 37.58,
            "unit": "ns/op\t      32 B/op\t       1 allocs/op",
            "extra": "28935784 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - ns/op",
            "value": 37.58,
            "unit": "ns/op",
            "extra": "28935784 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - B/op",
            "value": 32,
            "unit": "B/op",
            "extra": "28935784 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - allocs/op",
            "value": 1,
            "unit": "allocs/op",
            "extra": "28935784 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation",
            "value": 26.99,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "44975578 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - ns/op",
            "value": 26.99,
            "unit": "ns/op",
            "extra": "44975578 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "44975578 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "44975578 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet",
            "value": 183.8,
            "unit": "ns/op\t     336 B/op\t       3 allocs/op",
            "extra": "6552217 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - ns/op",
            "value": 183.8,
            "unit": "ns/op",
            "extra": "6552217 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - B/op",
            "value": 336,
            "unit": "B/op",
            "extra": "6552217 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - allocs/op",
            "value": 3,
            "unit": "allocs/op",
            "extra": "6552217 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "81800915+henrikrexed@users.noreply.github.com",
            "name": "Henrik Rexed",
            "username": "henrikrexed"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "abbd7b67cffe22937d3f4363522976cbc356d8ba",
          "message": "Merge pull request #35 from K8sAgentMesh/dependabot/docker/images/node-26-slim\n\nbuild(deps): Bump node from 22-slim to 26-slim in /images",
          "timestamp": "2026-10-25T18:08:47+01:00",
          "tree_id": "ef5e406bef946b1d4a5c0d8a5f982e842ef34a0a",
          "url": "https://github.com/K8sAgentMesh/K8sAgentMesh/commit/abbd7b67cffe22937d3f4363522976cbc356d8ba"
        },
        "date": 1792948163613,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkRoute",
            "value": 49.92,
            "unit": "ns/op\t      32 B/op\t       1 allocs/op",
            "extra": "23711674 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - ns/op",
            "value": 49.92,
            "unit": "ns/op",
            "extra": "23711674 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - B/op",
            "value": 32,
            "unit": "B/op",
            "extra": "23711674 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - allocs/op",
            "value": 1,
            "unit": "allocs/op",
            "extra": "23711674 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation",
            "value": 36.35,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "33005598 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - ns/op",
            "value": 36.35,
            "unit": "ns/op",
            "extra": "33005598 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "33005598 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "33005598 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet",
            "value": 249.3,
            "unit": "ns/op\t     336 B/op\t       3 allocs/op",
            "extra": "4781019 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - ns/op",
            "value": 249.3,
            "unit": "ns/op",
            "extra": "4781019 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - B/op",
            "value": 336,
            "unit": "B/op",
            "extra": "4781019 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - allocs/op",
            "value": 3,
            "unit": "allocs/op",
            "extra": "4781019 times\n4 procs"
          }
        ]
      },
      {
        "commit": {
          "author": {
            "email": "81800915+henrikrexed@users.noreply.github.com",
            "name": "Henrik Rexed",
            "username": "henrikrexed"
          },
          "committer": {
            "email": "noreply@github.com",
            "name": "GitHub",
            "username": "web-flow"
          },
          "distinct": true,
          "id": "be4436032b6989ffeece6acd39db34dfb1ccaf49",
          "message": "Merge pull request #36 from K8sAgentMesh/dependabot/docker/distroless/static-18f4cc2\n\nbuild(deps): Bump distroless/static from `e2e927e` to `18f4cc2`",
          "timestamp": "2026-10-25T18:09:30+01:00",
          "tree_id": "f3c3dcdc6cbe135984e2963e2fc8a4e39e43563c",
          "url": "https://github.com/K8sAgentMesh/K8sAgentMesh/commit/be4436032b6989ffeece6acd39db34dfb1ccaf49"
        },
        "date": 1792948444095,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkRoute",
            "value": 26.94,
            "unit": "ns/op\t      32 B/op\t       1 allocs/op",
            "extra": "44059222 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - ns/op",
            "value": 26.94,
            "unit": "ns/op",
            "extra": "44059222 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - B/op",
            "value": 32,
            "unit": "B/op",
            "extra": "44059222 times\n4 procs"
          },
          {
            "name": "BenchmarkRoute - allocs/op",
            "value": 1,
            "unit": "allocs/op",
            "extra": "44059222 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation",
            "value": 19.2,
            "unit": "ns/op\t       0 B/op\t       0 allocs/op",
            "extra": "63672823 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - ns/op",
            "value": 19.2,
            "unit": "ns/op",
            "extra": "63672823 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "63672823 times\n4 procs"
          },
          {
            "name": "BenchmarkIsSaturation - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "63672823 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet",
            "value": 137.6,
            "unit": "ns/op\t     336 B/op\t       3 allocs/op",
            "extra": "8624026 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - ns/op",
            "value": 137.6,
            "unit": "ns/op",
            "extra": "8624026 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - B/op",
            "value": 336,
            "unit": "B/op",
            "extra": "8624026 times\n4 procs"
          },
          {
            "name": "BenchmarkNewTeamSet - allocs/op",
            "value": 3,
            "unit": "allocs/op",
            "extra": "8624026 times\n4 procs"
          }
        ]
      }
    ]
  }
}