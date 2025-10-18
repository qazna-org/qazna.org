FROM rust:1.80-bookworm AS builder

WORKDIR /workspace

# Copy the Rust workspace manifests first to leverage build caching.
COPY core/Cargo.toml core/Cargo.toml
COPY core/Cargo.lock core/Cargo.lock
COPY core/ledger/Cargo.toml core/ledger/Cargo.toml

# Copy the full workspace sources.
COPY core /workspace/core
COPY api /workspace/api

RUN cd core && cargo build --release --bin ledgerd

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/lib/ledger && chown -R 65532:65532 /var/lib/ledger

COPY --from=builder /workspace/core/target/release/ledgerd /usr/local/bin/ledgerd

EXPOSE 9091
EXPOSE 9102

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/ledgerd"]
CMD ["--addr", "0.0.0.0:9091", "--metrics-addr", "0.0.0.0:9102"]
