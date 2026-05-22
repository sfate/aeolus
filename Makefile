.PHONY: run build lint audit test ci

TEST_DIR ?= ./...
TEST_CASE ?= ^.+$
BINARY_NAME ?= aeolus
BUILD_OUTPUT ?= bin/$(BINARY_NAME)

run:
	go run .

build:
	mkdir -p $(dir $(BUILD_OUTPUT))
	go build -mod=readonly -o $(BUILD_OUTPUT) .

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0 run --allow-parallel-runners

audit:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test:
	go test -mod=readonly -count=1 -p 1 -failfast -race -run $(TEST_CASE) $(TEST_DIR)

ci: lint audit test
