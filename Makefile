
GHCR_IMAGE ?= ghcr.io/dominikpalatynski/prout
IMAGE_TAG ?= latest
IMAGE_SHA_TAG ?= $(shell git rev-parse --short HEAD)

build:
	go build -o ./bin/prout cmd/main.go
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./bin/prout cmd/main.go

run:
	./bin/prout server --config ./server.yml

docker-build:
	docker compose -f ./docker-compose.prout.yml build

docker-up:
	docker compose -f ./docker-compose.prout.yml up -d --build

docker-logs:
	docker compose -f ./docker-compose.prout.yml logs -f prout

docker-down:
	docker compose -f ./docker-compose.prout.yml down

docker-push:
	docker build -f ./Dockerfile -t $(GHCR_IMAGE):$(IMAGE_TAG) -t $(GHCR_IMAGE):$(IMAGE_SHA_TAG) .
	docker push $(GHCR_IMAGE):$(IMAGE_TAG)
	docker push $(GHCR_IMAGE):$(IMAGE_SHA_TAG)
