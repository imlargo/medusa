.PHONY: dev run worker migrate test vet build sqlc clean

# Code generators run from pinned images, so the generated code does not
# depend on what is installed on the machine that ran them.
SQLC_IMAGE := sqlc/sqlc:1.30.0
DOCKER_RUN := docker run --rm -u "$$(id -u):$$(id -g)"

dev:
	air

run:
	go run ./cmd/api

# Runs the worker that executes queued jobs.
worker:
	go run ./cmd/worker

# Applies pending migrations to DATABASE_URL.
migrate:
	go run ./cmd/migrate

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -o ./bin/api ./cmd/api
	go build -o ./bin/migrate ./cmd/migrate
	go build -o ./bin/worker ./cmd/worker

# Regenerates every feature's internal/db from its migrations and queries.
sqlc:
	$(DOCKER_RUN) -v "$(CURDIR):/src" -w /src $(SQLC_IMAGE) generate

clean:
	rm -rf ./bin ./tmp coverage.out coverage.html
