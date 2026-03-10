# === Frontend stage ===
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend

WORKDIR /src

# Cache frontend dependencies
COPY web/package.json web/package-lock.json ./web/
RUN npm --prefix web ci

# Copy frontend source (includes pre-generated proto TS in web/src/gen/)
COPY web/ ./web/

# Build frontend assets
RUN npm --prefix web run build

# === Go builder stage ===
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS builder

WORKDIR /src

# Cache Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source (pre-generated proto and sqlc files are included)
COPY . .

# Copy built frontend assets from frontend stage
COPY --from=frontend /src/internal/server/ui/dist/ ./internal/server/ui/dist/

# Build server binary (amd64 for Cloud Run)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/server ./cmd/server

# Build arcad binaries for both architectures
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/arcad/arcad_linux_amd64 ./cmd/arcad
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o /out/arcad/arcad_linux_arm64 ./cmd/arcad

# === Runtime stage ===
FROM --platform=linux/amd64 gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/server /usr/local/bin/server
COPY --from=builder /out/arcad/ /opt/arcad/

ENV ARCAD_BINARY_DIR=/opt/arcad
EXPOSE 8080

ENTRYPOINT ["server"]
