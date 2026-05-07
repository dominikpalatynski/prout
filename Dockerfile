# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN set -eu; \
	export GOOS="${TARGETOS:-linux}"; \
	if [ -n "${TARGETARCH:-}" ]; then export GOARCH="${TARGETARCH}"; fi; \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/prout ./cmd/main.go

FROM docker:28-cli

WORKDIR /opt/prout

COPY --from=build /out/prout /usr/local/bin/prout

ENTRYPOINT ["prout"]
CMD ["server", "--config", "/opt/prout/server.yml"]
