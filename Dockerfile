# syntax=docker/dockerfile:1
FROM debian:stable-slim AS base

# Install minimal dependencies (if any are needed)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user (optional, for security)
RUN useradd -m llama

# Create required directories
RUN mkdir -p /usr/local/bin /var/lib /var/run && chown llama:llama /var/run

# Copy the prebuilt llama-server binary (mock for now)
COPY --chown=llama:llama llama-server /usr/local/bin/llama-server

# Copy the model file (mock for now)
COPY --chown=llama:llama mixtral.gguf /var/lib/mixtral.gguf

# Set environment variables from llama-server.service
ENV OLLAMA_NUM_GPU=999 \
    ZES_ENABLE_SYSMAN=1 \
    SYCL_CACHE_PERSISTENT=1 \
    OLLAMA_KEEP_ALIVE=10m \
    SYCL_PI_LEVEL_ZERO_USE_IMMEDIATE_COMMANDLISTS=1

# Expose the Unix socket path as a VOLUME (for host access, if needed)
VOLUME ["/var/run"]

# Ensure the socket file does not exist before starting
RUN rm -f /var/run/llama.sock || true

# Switch to non-root user
USER llama

# Entrypoint: run the server with the correct arguments
ENTRYPOINT ["/usr/local/bin/llama-server", "--model", "/var/lib/mixtral.gguf", "--host", "/var/run/llama.sock"]