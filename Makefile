MIGRATIONS_PATH=internal/db/migrations

include .env
export $(shell sed 's/=.*//' .env)


migrate-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" down

migrate-create:
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $(name)