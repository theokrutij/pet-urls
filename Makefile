run-app:
	go run ./cmd/app

COMPOSE_FILE := docker/compose.dev.yml
VOLUME_NAME := $(shell docker volume ls -q --filter name=postgres_data)


compose-up:
	docker compose -f $(COMPOSE_FILE) up

compose-down:
	docker compose -f $(COMPOSE_FILE) down

compose-restart: compose-down compose-up

compose-logs:
	docker compose -f $(COMPOSE_FILE) logs -f

compose-ps:
	docker compose -f $(COMPOSE_FILE) ps

compose-build:
	docker compose -f $(COMPOSE_FILE) build

compose-exec:
	docker compose -f $(COMPOSE_FILE) exec db bash

volume-rm:
	@if [ -n "$(VOLUME_NAME)" ]; then \
		docker volume rm $(VOLUME_NAME); \
	else \
		echo "Volume 'postgres_data' does not exist."; \
	fi


# Migration config
MIGRATIONS_DIR=./internal/db/postgres/migrations
DB_DSN=postgres://postgres:dev-password@localhost:5432/postgres?sslmode=disable

# Create a new migration
create-migration:
	@if [ -z "$(name)" ]; then \
		echo "Usage: make create-migration name=your_migration_name"; \
		exit 1; \
	fi
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

# Run up migrations
migrate-up:
	go run ./cmd/migrate/main.go up -dsn "$(DB_DSN)" -dir "$(MIGRATIONS_DIR)"

# Roll back last migration
migrate-down:
	go run cmd/migrate/main.go down -dsn "$(DB_DSN)" -dir "$(MIGRATIONS_DIR)"

# Force set migration version (usage: make migrate-force version=3)
migrate-force:
	@if [ -z "$(version)" ]; then \
		echo "Usage: make migrate-force version=N"; \
		exit 1; \
	fi
	go run cmd/migrate/main.go force -dsn "$(DB_DSN)" -dir "$(MIGRATIONS_DIR)" $(version)

# Show current migration version
migrate-version:
	go run cmd/migrate/main.go version -dsn "$(DB_DSN)" -dir "$(MIGRATIONS_DIR)"