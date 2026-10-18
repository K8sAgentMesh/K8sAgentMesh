# lane-vibe — TEST-ONLY merge of the two real images the vanilla chart
# composes via its toolchain initContainer (agentruntime supervisor +
# toolchain-vibe CLI), so a one-shot `kubectl run -i` pod can drive the
# supervisor's stdin/stdout /task NDJSON contract with the exact bits the
# chart ships. Not referenced by the chart; built only by
# .github/workflows/live-validation.yml.
FROM ghcr.io/k8sagentmesh/toolchain-vibe:0.1.0-lv
COPY --from=ghcr.io/k8sagentmesh/agentruntime:0.1.0-lv \
     /usr/local/bin/agentruntime /usr/local/bin/agentruntime
# toolchain-vibe already sets PATH=/root/.local/bin (the uv-installed vibe
# CLI) and WORKDIR /workspace.
ENTRYPOINT ["agentruntime"]
