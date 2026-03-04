FROM cloudflare/cloudflared:latest AS cloudflared

FROM golang:1.24 AS arcad-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/arcad ./cmd/arcad

FROM node:22-bookworm-slim AS node-runtime

FROM node:22-bookworm-slim AS claudecodeui-builder
ARG CLAUDECODEUI_ZIP_URL="https://github.com/ryotarai/claudecodeui/archive/refs/tags/ryotarai-v1.21.0-2.zip"
ENV VITE_BASE_PATH="/__arca/claudecodeui"
ENV VITE_IS_PLATFORM=true

WORKDIR /work
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl unzip python3 make g++ \
  && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "$CLAUDECODEUI_ZIP_URL" -o app.zip \
  && unzip app.zip \
  && rm app.zip \
  && mv claudecodeui-* app

WORKDIR /work/app
RUN npm ci
RUN npm run build

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV BASE_PATH="/__arca/claudecodeui"
ENV PORT=21031
ENV VITE_IS_PLATFORM=true

RUN apt-get update \
  && apt-get install -y --no-install-recommends bash ca-certificates curl dumb-init git jq python3 tmux ttyd \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --create-home --home-dir /home/arca --shell /bin/bash arca

COPY --from=node-runtime /usr/local /usr/local
COPY --from=cloudflared /usr/local/bin/cloudflared /usr/local/bin/cloudflared
COPY --from=arcad-builder /out/arcad /usr/local/bin/arcad
COPY --from=claudecodeui-builder /work/app /home/arca/claudecodeui
COPY docker/machine/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN su - arca -c 'curl -fsSL https://claude.ai/install.sh | bash' || npm i -g @anthropic-ai/claude-code \
  && npm i -g @openai/codex \
  && if [ -x /home/arca/.local/bin/claude ]; then ln -sf /home/arca/.local/bin/claude /usr/local/bin/claude; fi \
  && mkdir -p /workspace \
  && chown arca:arca /workspace \
  && chmod 700 /workspace \
  && chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/arcad \
  && chown -R arca:arca /home/arca/claudecodeui

USER arca
WORKDIR /home/arca

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/entrypoint.sh"]
