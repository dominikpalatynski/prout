# syntax=docker/dockerfile:1.7

# ============================================
# Stage 1: tailwind binary
# ============================================
FROM alpine:3.19 AS tailwind
ARG TAILWIND_VERSION=v3.4.17
ARG TARGETARCH
RUN apk add --no-cache curl \
 && curl -sSL "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${TARGETARCH}" -o /tailwindcss \
 && chmod +x /tailwindcss

# ============================================
# Stage 2: build
# ============================================
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY --from=tailwind /tailwindcss /usr/local/bin/tailwindcss
COPY . .

ARG VERSION=dev
RUN go tool templ generate \
 && go tool sqlc generate \
 && tailwindcss \
      -i ./internal/web/styles/input.css \
      -o ./internal/web/static/output.css \
      --minify

RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/toolshed \
      ./cmd/toolshed

# ============================================
# Stage 3: runtime
# ============================================
FROM alpine:3.19 AS runtime
RUN apk add --no-cache \
      docker-cli \
      docker-cli-compose \
      ca-certificates \
      tzdata \
 && addgroup -S toolshed \
 && adduser -S toolshed -G toolshed -G docker

COPY --from=build /out/toolshed /usr/local/bin/toolshed

USER toolshed
WORKDIR /var/lib/toolshed
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/toolshed"]
CMD ["server"]
