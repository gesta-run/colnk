.PHONY: build build-macos build-linux test test-linux docker-up docker-down clean

build: build-macos build-linux

build-macos:
	mkdir -p bin
	GOTOOLCHAIN=local go build -o bin/colnk ./cmd/colnk
	GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/colnk-darwin-arm64 ./cmd/colnk
	GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o bin/colnk-darwin-amd64 ./cmd/colnk

build-linux:
	mkdir -p bin
	GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/colnk-server-linux-amd64 ./cmd/colnk-server

test:
	GOTOOLCHAIN=local go test ./...

test-linux:
	linux_test_dir=$$(mktemp -d); trap 'rm -r "$$linux_test_dir"' EXIT; \
	GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o "$$linux_test_dir/filesystem.test" ./pkg/agent/filesystem; \
	GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o "$$linux_test_dir/network.test" ./pkg/agent/network; \
	GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o "$$linux_test_dir/provider-filesystem.test" ./pkg/provider/filesystem; \
	GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$$linux_test_dir/colnk-server" ./cmd/colnk-server

docker-up:
	./scripts/init-dev-secrets.sh
	docker compose up -d --build

docker-down:
	docker compose down

clean:
	rm -rf bin
