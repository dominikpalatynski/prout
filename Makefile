
build:
	go build -o ./bin/prout cmd/main.go
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./bin/prout cmd/main.go

run:
	./bin/prout server --config ./server.yml