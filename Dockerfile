# syntax=docker/dockerfile:1
FROM debian:stable-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libcurl4 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create llama user
RUN useradd -m llama

# Create required directories
RUN mkdir -p /usr/local/bin /usr/local/lib /var/lib /var/run && chown llama:llama /usr/local/bin /usr/local/lib /var/lib /var/run

# Copy in the prebuilt binaries and model, set ownership
COPY --chown=llama:llama llama-server /usr/local/bin/llama-server
COPY --chown=llama:llama libmtmd.so /usr/local/lib/libmtmd.so
COPY --chown=llama:llama mixtral.gguf /var/lib/mixtral.gguf

# Set LD_LIBRARY_PATH so llama-server can find libmtmd.so
ENV LD_LIBRARY_PATH=/usr/local/lib

# Switch to non-root user
USER llama

# Entrypoint for the llama-server
ENTRYPOINT ["/usr/local/bin/llama-server", "--model", "/var/lib/mixtral.gguf", "--host", "/var/run/llama.sock"]