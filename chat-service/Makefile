.PHONY: run dev build test tidy clean

# One-shot run (no reload)
run:
	go run ./cmd/server

# Hot reload via air (installs air on first use if missing from PATH)
dev:
	@if command -v air >/dev/null 2>&1; then \
		air -c .air.toml; \
	else \
		echo "air not in PATH — using go run github.com/air-verse/air@v1.61.7"; \
		go run github.com/air-verse/air@v1.61.7 -c .air.toml; \
	fi

build:
	go build -o ./tmp/ws-server ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf ./tmp
