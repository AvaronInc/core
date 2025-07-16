# syntax=docker/dockerfile:1
FROM debian:stable

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libcurl4 \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

# Create llama user
RUN useradd -m llama

# Create required directories
RUN mkdir -p /usr/local/bin /usr/local/lib /var/lib /var/run && chown llama:llama /usr/local/bin /usr/local/lib /var/lib /var/run

# Copy shared libraries to /usr/local/lib
COPY --chown=llama:llama libmtmd.so /usr/local/lib/libmtmd.so
COPY --chown=llama:llama libllama.so /usr/local/lib/libllama.so
COPY --chown=llama:llama libggml.so /usr/local/lib/libggml.so
COPY --chown=llama:llama libggml-base.so /usr/local/lib/libggml-base.so

# Copy prebuilt llama-server binary to /usr/local/bin
COPY --chown=llama:llama llama-server /usr/local/bin/llama-server

# Copy model file to /var/lib
COPY --chown=llama:llama mixtral.gguf /var/lib/mixtral.gguf

# Set LD_LIBRARY_PATH to include /usr/local/lib
ENV LD_LIBRARY_PATH=/usr/local/lib:/usr/lib/x86_64-linux-gnu

# Switch to non-root user
USER llama

# Entrypoint for the llama-server
ENTRYPOINT ["/usr/local/bin/llama-server", "--model", "/var/lib/mixtral.gguf", "--host", "/var/run/llama.sock"]