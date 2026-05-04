
build:
	go build -o ./bin/toolshed cmd/main.go
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./bin/toolshed cmd/main.go

run:
	./bin/toolshed server --config ./server.yml