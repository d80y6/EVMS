up:
	docker compose up -d --build

down:
	docker compose down -v

health:
	./scripts/health-check.sh

ci-gate:
	./scripts/ci-gate.sh
