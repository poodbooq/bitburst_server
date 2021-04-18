.PHONY: up down stop build logs logs-server logs-tester logs-postgres stats reup

up:
	docker compose up -d

down:
	docker compose down

stop:
	docker compose stop

build:
	docker compose build --no-cache

logs:
	docker compose logs -f

logs-server:
	docker compose logs server -f

logs-tester:
	docker compose logs tester -f

logs-postgres:
	docker compose logs postgres -f

stats:
	docker stats

reup: down build up
