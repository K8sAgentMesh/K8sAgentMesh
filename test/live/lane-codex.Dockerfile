# lane-codex — TEST-ONLY codex substrate for the ISI-6211 live validation
# lane. Plan decision D2 deferred the codex toolchain image, and the
# `ksquad-codex-exec` D4 wrapper this repo's codex engine execs is not
# vendored here (it lives in the k8squad host project); this fixture builds
# an equivalent substrate so the mesh-side wire + settle contract is still
# proven. Not referenced by the chart.
FROM node:22-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git \
 && rm -rf /var/lib/apt/lists/*
RUN npm install -g @openai/codex \
 && codex --version
COPY --from=ghcr.io/k8sagentmesh/agentruntime:0.1.0-lv \
     /usr/local/bin/agentruntime /usr/local/bin/agentruntime
COPY test/live/mesh-codex-exec /usr/local/bin/ksquad-codex-exec
RUN chmod 0755 /usr/local/bin/ksquad-codex-exec
ENV PATH="/usr/local/bin:${PATH}"
WORKDIR /workspace
ENTRYPOINT ["agentruntime"]
