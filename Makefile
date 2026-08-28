.PHONY: dev health smoke test race verify release production-check backup shellcheck readiness-test dev-reset-plan dev-full-reset-plan dev-reset-sentinel-plan

dev:
	bash scripts/local-dev.sh

health:
	bash scripts/local-health.sh

smoke:
	bash scripts/local-smoke.sh

test:
	cd backend && go test ./...
	cd new && npm run test
	cd new-back && npm run test

race:
	cd backend && go test -race ./...

verify: test
	cd new && npm run lint && npm run build
	cd new-back && npm run lint && npm run build

release: verify
	rm -rf release
	mkdir -p release/bin release/member release/admin release/deploy release/scripts/lib
	cd backend && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../release/bin/wangzhe-backend .
	cd backend && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../release/bin/wangzhe-bootstrap-admin ./cmd/bootstrap-admin
	cp -R new/dist/. release/member/
	cp -R new-back/dist/. release/admin/
	cp -R deploy/. release/deploy/
	cp scripts/production-readiness.sh scripts/postgres-backup.sh release/scripts/
	cp scripts/lib/backend-env.sh release/scripts/lib/
	cp PRODUCTION_OPERATIONS.md release/

production-check:
	bash scripts/production-readiness.sh

backup:
	bash scripts/postgres-backup.sh

shellcheck:
	bash -n scripts/*.sh scripts/lib/*.sh
	! rg -n '127\.0\.0\.1:8089|BACKEND_SERVER_ALLOWED_ORIGINS=.*http://' deploy

readiness-test: shellcheck
	bash scripts/readiness-static-test.sh
	bash scripts/dev-reset-static-test.sh

dev-reset-plan:
	@if [ -n "$(ENV_FILE)" ]; then \
		bash scripts/dev-reset-business-data.sh --dry-run "$(ENV_FILE)"; \
	else \
		bash scripts/dev-reset-business-data.sh --dry-run; \
	fi

dev-full-reset-plan:
	@if [ -n "$(ENV_FILE)" ]; then \
		bash scripts/dev-reset-database.sh --dry-run "$(ENV_FILE)"; \
	else \
		bash scripts/dev-reset-database.sh --dry-run; \
	fi

dev-reset-sentinel-plan:
	@if [ -n "$(ENV_FILE)" ]; then \
		bash scripts/dev-reset-init-sentinel.sh --dry-run "$(ENV_FILE)"; \
	else \
		bash scripts/dev-reset-init-sentinel.sh --dry-run; \
	fi
