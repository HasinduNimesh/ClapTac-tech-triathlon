MODULE := github.com/HasinduNimesh/ClapTac-tech-triathlon
SERVICES := order-service planning-service fleet-service loading-service delivery-service shared-service integration-service agent-orchestrator

.PHONY: build test verify web-install web-build mobile-analyze compose-up compose-down compose-observability compose-agent-traces validate-observability verify-object-storage-restore backup-postgres backup-object-storage lint check-agent-imports seed-competition-data validate-seeds

build:
	go build ./...

test:
	go test ./...

verify:
	./scripts/test.sh

web-install:
	cd apps/web && npm install

web-build: web-install
	cd apps/web && npm run build

mobile-analyze:
	cd apps/driver-mobile && flutter analyze

compose-up:
	@docker compose version >/dev/null 2>&1 || { echo "Install the Compose v2 plugin (brew install docker-compose) so 'docker compose' works."; exit 1; }
	docker compose up --build

compose-down:
	docker compose down

compose-observability:
	docker compose --profile observability up --build

compose-agent-traces:
	AGENT_TRACE_URL=http://agent-manager:8085 docker compose --profile agent-traces up --build

validate-observability:
	./scripts/test-observability.sh

verify-object-storage-restore:
	./scripts/test-object-storage-restore.sh

backup-postgres:
	./scripts/create-postgres-backup.sh

backup-object-storage:
	./scripts/create-object-storage-backup.sh

lint:
	go vet ./...

check-agent-imports:
	@./scripts/check-agent-imports.sh

seed-competition-data:
	./scripts/validate-seeds.py
	./scripts/import-datasets.sh

validate-seeds:
	./scripts/validate-seeds.py
