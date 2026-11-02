.PHONY: dev run migrate test vet build sqlc docs clean

# Code generators run from pinned images, so the generated code does not
# depend on what is installed on the machine that ran them.
SQLC_IMAGE := sqlc/sqlc:1.30.0
SWAG_IMAGE := ghcr.io/swaggo/swag:v1.16.6
DOCKER_RUN := docker run --rm -u "$$(id -u):$$(id -g)"

dev:
	air

run:
	go run ./cmd/api

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

# Regenerates internal/database/sqlc from the migrations and the queries.
sqlc:
	$(DOCKER_RUN) -v "$(CURDIR):/src" -w /src $(SQLC_IMAGE) generate

# Regenerates api/docs from the handler annotations. The repo root is mounted
# because the response types live in the Medusa module one level up.
docs:
	$(DOCKER_RUN) -v "$(CURDIR)/..:/src" -w /src/app $(SWAG_IMAGE) init -g cmd/api/main.go --parseDependency --parseInternal -o api/docs

clean:
	rm -rf ./bin ./tmp coverage.out coverage.html
