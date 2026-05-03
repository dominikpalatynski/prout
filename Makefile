
build:
	go build -o ./bin/toolshed cmd/main.go

run:
	./bin/toolshed server --config ./config.yml