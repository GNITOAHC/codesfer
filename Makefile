all: cli server

cli:
	go build -o build/codesfer ./cmd/codesfer/main.go

server:
	go build -o build/codeserver ./cmd/codesfer-server/main.go

