# lane-vibe — TEST-ONLY vibe substrate for the ISI-6211 live validation
# lane. The product image (images/Dockerfile.toolchain-vibe) pins
# `vibe==0.3.0`, a pin that exists on no PyPI (the CLI ships as
# `mistral-vibe`, whose versions are 0.0.0 then 1.0.0+), so it has never
# been buildable — the lane builds its own substrate from the real
# package instead, at the current latest, to prove the mesh-side wire and
# settle contract. Not referenced by the chart.
FROM python:3.12-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git ripgrep \
 && rm -rf /var/lib/apt/lists/*
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv
RUN uv tool install --python-preference only-system mistral-vibe==2.9.6 \
 && uv tool update-shell || true
ENV PATH="/root/.local/bin:${PATH}" \
    VIBE_NONINTERACTIVE=1
RUN vibe --version
COPY --from=ghcr.io/k8sagentmesh/agentruntime:0.1.0-lv \
     /usr/local/bin/agentruntime /usr/local/bin/agentruntime
WORKDIR /workspace
ENTRYPOINT ["agentruntime"]
