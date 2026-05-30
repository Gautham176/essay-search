.PHONY: demo down clean test help

help:
	@echo "Targets:"
	@echo "  make demo   - one-shot setup: containers + corpus + index"
	@echo "  make down   - stop containers, keep data"
	@echo "  make clean  - stop containers, wipe all data and corpus"
	@echo "  make test   - run all Go tests"

demo:
	@bash scripts/bootstrap.sh

down:
	docker compose down

clean:
	docker compose down -v
	rm -rf corpus/clean scripts/.venv

test:
	go test ./...