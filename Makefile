.PHONY: dev health smoke test race verify release release-contract-check production-check production-config-check backup upload-backup backup-integrity monitor restore-drill pitr-restore-drill shellcheck readiness-test rclone-integration-test dev-reset-plan dev-full-reset-plan dev-reset-sentinel-plan production-test-install production-system-test integration-test catalog-integration-test timing-integration-test e2e-test load-test

RELEASE_GOOS ?= linux
RELEASE_GOARCH ?= amd64

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
	@if rg -n '123456|Admin8801!|Wz888888|WzTenant8801|Room8801' new/dist new-back/dist; then \
		echo "生产前端产物包含本地体验密码，拒绝继续" >&2; \
		exit 1; \
	fi

release: verify readiness-test
	@case "$(RELEASE_GOOS)/$(RELEASE_GOARCH)" in linux/amd64|linux/arm64) ;; *) echo "release 仅支持 linux/amd64 或 linux/arm64" >&2; exit 1;; esac
	rm -rf release
	mkdir -p release/bin release/member release/admin release/deploy release/scripts/lib
	cd backend && CGO_ENABLED=0 GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH) go build -trimpath -ldflags="-s -w" -o ../release/bin/wangzhe-backend .
	cd backend && CGO_ENABLED=0 GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH) go build -trimpath -ldflags="-s -w" -o ../release/bin/wangzhe-bootstrap-admin ./cmd/bootstrap-admin
	cd backend && CGO_ENABLED=0 GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH) go build -trimpath -ldflags="-s -w" -o ../release/bin/wangzhe-test-site-accounts ./cmd/test-site-accounts
	printf '%s/%s\n' '$(RELEASE_GOOS)' '$(RELEASE_GOARCH)' > release/TARGET
	cp -R new/dist/. release/member/
	cp -R new-back/dist/. release/admin/
	cp -R deploy/. release/deploy/
	cp scripts/production-config-check.sh scripts/production-readiness.sh scripts/production-deploy.sh scripts/production-rollback.sh \
		scripts/postgres-backup.sh scripts/upload-backup.sh scripts/postgres-archive-wal.sh scripts/postgres-base-backup.sh \
		scripts/postgres-restore-wal.sh scripts/production-restore-drill.sh scripts/production-monitor.sh scripts/production-backup-integrity.sh \
		scripts/pitr-recovery-source-sync.sh scripts/production-pitr-restore-drill.sh scripts/publish-pitr-drill-status.sh \
		scripts/production-unit-failure-alert.sh scripts/production-recovery-evidence-check.sh \
		scripts/redis-production-check.sh scripts/release-integrity.sh release/scripts/
	cp scripts/lib/backend-env.sh scripts/lib/safe-integer.sh scripts/lib/maintenance-edge.sh \
		scripts/lib/strict-env.sh scripts/lib/encrypted-backup.sh release/scripts/lib/
	cp PRODUCTION_OPERATIONS.md release/
	test -f release/scripts/production-restore-drill.sh -a -f release/scripts/production-recovery-evidence-check.sh \
		-a -f release/deploy/env/restore-drill.env.example -a -f release/deploy/env/recovery-evidence.env.example \
		-a -f release/deploy/systemd/wangzhe-restore-drill.service
	$(MAKE) release-contract-check
	bash scripts/release-integrity.sh generate release
	@{ command -v sha256sum >/dev/null 2>&1 && sha256sum release/SHA256SUMS || shasum -a 256 release/SHA256SUMS; } | awk '{print "可信清单摘要（部署时使用 EXPECTED_MANIFEST_SHA256）：" $$1}'

# Keep the generated member bundle and backend binary on the same betting
# contract. This fails the build if an old member release or pre-mark6 backend
# is accidentally packaged after the source has already enabled the board.
release-contract-check:
	@test -x release/bin/wangzhe-backend || { echo "发布包缺少后端二进制" >&2; exit 1; }
	@grep -aFq 'mark6-v1' release/bin/wangzhe-backend || { echo "后端发布包缺少历史 mark6-v1" >&2; exit 1; }
	@grep -aFq 'mark6-v2' release/bin/wangzhe-backend || { echo "后端发布包缺少当前 mark6-v2" >&2; exit 1; }
	@grep -aFRq 'mark-six-bet-board' release/member/assets || { echo "会员端发布包缺少宾果六合彩网投面板" >&2; exit 1; }
	@grep -aFRq 'web-bets' release/member/assets || { echo "会员端发布包缺少批量网投接口" >&2; exit 1; }

production-check:
	bash scripts/production-readiness.sh

production-config-check:
	bash scripts/production-config-check.sh

backup:
	bash scripts/postgres-backup.sh

upload-backup:
	bash scripts/upload-backup.sh

backup-integrity:
	bash scripts/production-backup-integrity.sh

monitor:
	bash scripts/production-monitor.sh

restore-drill:
	bash scripts/production-restore-drill.sh

pitr-restore-drill:
	bash scripts/production-pitr-restore-drill.sh

shellcheck:
	bash -n scripts/*.sh scripts/lib/*.sh
	@if command -v shellcheck >/dev/null 2>&1; then \
		shellcheck -S warning scripts/*.sh scripts/lib/*.sh; \
	elif [ "$${CI:-}" = "true" ]; then \
		echo "CI 缺少 ShellCheck，拒绝跳过" >&2; exit 1; \
	else \
		echo "shellcheck 未安装，已完成 bash -n；生产构建机应安装 shellcheck"; \
	fi
	! rg -n '127\.0\.0\.1:8089|BACKEND_SERVER_ALLOWED_ORIGINS=.*http://' deploy

readiness-test: shellcheck
	bash scripts/readiness-static-test.sh
	bash scripts/ops-resilience-static-test.sh
	bash scripts/recovery-evidence-check-static-test.sh
	bash scripts/dev-reset-static-test.sh

rclone-integration-test:
	bash scripts/rclone-offsite-integration-test.sh

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

production-test-install:
	cd new && npm ci --ignore-scripts
	cd new-back && npm ci --ignore-scripts
	cd tests/e2e && npm ci --ignore-scripts
	cd tests/system && npm ci --ignore-scripts
	cd tests/e2e && npx playwright install --with-deps chromium

production-system-test:
	SYSTEM_TEST_SUITE=all bash scripts/release-system-test.sh

integration-test:
	SYSTEM_TEST_SUITE=integration bash scripts/release-system-test.sh

catalog-integration-test:
	@test -n "$$BACKEND_CATALOG_TEST_DSN" || { echo "请设置独立空库 BACKEND_CATALOG_TEST_DSN" >&2; exit 1; }
	cd backend && go test . -run '^TestCatalogDefaults(Fresh|Upgrade)Postgres$$' -count=1 -v

timing-integration-test:
	@test -n "$$BACKEND_TIMING_TEST_DSN" || { echo "请设置独立空库 BACKEND_TIMING_TEST_DSN" >&2; exit 1; }
	cd backend && go test ./services -run '^TestLotteryTimingPostgres' -count=1 -v

e2e-test:
	SYSTEM_TEST_SUITE=e2e bash scripts/release-system-test.sh

load-test:
	SYSTEM_TEST_SUITE=load bash scripts/release-system-test.sh
