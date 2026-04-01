FROM rust:1.82-slim-bookworm AS builder

RUN apt-get update && apt-get install -y pkg-config libssl-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY Cargo.toml Cargo.lock ./
COPY worker/ worker/
COPY cli/ cli/

RUN cargo build --release -p km-worker

# ── Runtime ──────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/km-worker /usr/local/bin/km-worker

# Worker listens on 0.0.0.0 in Docker
ENV KM_BIND=0.0.0.0
ENV KM_PORT=8000

EXPOSE 8000

ENTRYPOINT ["km-worker"]
