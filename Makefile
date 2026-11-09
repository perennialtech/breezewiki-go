.PHONY: ci test build generate-config-examples check-config-examples
ci: test build check-config-examples

test:
	go test ./...
	go vet ./...

build:
	go build ./...

generate-config-examples:
	go run ./cmd/breezewiki -example-config ini > config.example.ini
	go run ./cmd/breezewiki -example-config env > .env.example

check-config-examples:
	go run ./cmd/breezewiki -example-config ini | diff -u config.example.ini -
	go run ./cmd/breezewiki -example-config env | diff -u .env.example -
