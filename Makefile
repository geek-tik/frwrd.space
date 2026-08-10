.PHONY: up down build logs ps migrate test lint

up:
	docker compose up -d --build

down:
	docker compose down

build:
	docker compose build

logs:
	docker compose logs -f

ps:
	docker compose ps

migrate:
	docker compose run --rm --entrypoint /app/migrate migrate up

test:
	go test ./...

lint:
	golangci-lint run ./... 2>/dev/null || go vet ./...

tunnel:
	docker compose --profile tunnel run --rm agent http $(PORT)
