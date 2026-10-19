# lane-vibe — TEST-ONLY vibe substrate for the ISI-6211 live validation
# lane. Two accommodations, both documented in test/live/README.md:
#   1. The product image (images/Dockerfile.toolchain-vibe) pins
#      `vibe==0.3.0`, a pin that exists on no PyPI (the CLI ships as
#      `mistral-vibe`, versions 0.0.0 -> 1.0.0+), so it has never been
#      buildable. The lane builds from the real package instead.
#   2. The agentruntime engine's vibe argv carries `--auto-approve`, a
#      flag of the k8squad-internal py-v0.3.0 lineage; public
#      mistral-vibe has no such flag (its -p mode auto-approves by
#      design, per its own --help). The shim below drops the flag.
FROM python:3.12-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git ripgrep \
 && rm -rf /var/lib/apt/lists/*
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv
RUN uv tool install --python-preference only-system mistral-vibe==2.9.6 \
 && uv tool update-shell || true
RUN mv /root/.local/bin/vibe /root/.local/bin/vibe.real \
 && printf '#!/usr/bin/env python3\nimport sys, os\nargs = [a for a in sys.argv[1:] if a != "--auto-approve"]\nos.execvp("vibe.real", ["vibe.real"] + args)\n' > /root/.local/bin/vibe \
 && chmod 0755 /root/.local/bin/vibe
ENV PATH="/root/.local/bin:${PATH}" \
    VIBE_NONINTERACTIVE=1
RUN vibe --version
COPY --from=ghcr.io/k8sagentmesh/agentruntime:0.1.0-lv \
     /usr/local/bin/agentruntime /usr/local/bin/agentruntime
WORKDIR /workspace
ENTRYPOINT ["agentruntime"]
