FROM node:22-bookworm-slim AS builder

ARG APP_ZIP_URL="https://github.com/ryotarai/claudecodeui/archive/refs/tags/ryotarai-v1.21.0-2.zip"
ENV VITE_BASE_PATH="/__arca/claudecodeui"

WORKDIR /work

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl unzip \
  && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "$APP_ZIP_URL" -o app.zip \
  && unzip app.zip \
  && rm app.zip \
  && mv claudecodeui-* app

WORKDIR /work/app
RUN npm ci
RUN npm run build

FROM node:22-bookworm-slim AS runtime

ENV NODE_ENV=production
ENV BASE_PATH="/__arca/claudecodeui"
ENV PORT=21031

WORKDIR /app
COPY --from=builder /work/app /app

EXPOSE 21031
CMD ["node", "server/index.js"]
