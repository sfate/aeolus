.PHONY: run build lint audit test ci

TEST_DIR ?= ./...
TEST_CASE ?= ^.+$

run:
	go run .

build:
	go build -o aeolus .

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0 run --allow-parallel-runners

audit:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test:
	go test -mod=readonly -count=1 -p 1 -failfast -race -run $(TEST_CASE) $(TEST_DIR)

ci: lint audit test
