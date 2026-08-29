.PHONY: dev health smoke test race verify release production-check production-config-check backup shellcheck readiness-test dev-reset-plan dev-full-reset-plan dev-reset-sentinel-plan

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
	@if rg -n '123456|Wz888888|WzTenant8801|Room8801' new/dist new-back/dist; then \
		echo "生产前端产物包含本地体验密码，拒绝继续" >&2; \
		exit 1; \
	fi

release: verify readiness-test
	@case "$(RELEASE_GOOS)/$(RELEASE_GOARCH)" in linux/amd64|linux/arm64) ;; *) echo "release 仅支持 linux/amd64 或 linux/arm64" >&2; exit 1;; esac
	rm -rf release
	mkdir -p release/bin release/member release/admin release/deploy release/scripts/lib
	cd backend && CGO_ENABLED=0 GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH) go build -trimpath -ldflags="-s -w" -o ../release/bin/wangzhe-backend .
	cd backend && CGO_ENABLED=0 GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH) go build -trimpath -ldflags="-s -w" -o ../release/bin/wangzhe-bootstrap-admin ./cmd/bootstrap-admin
	printf '%s/%s\n' '$(RELEASE_GOOS)' '$(RELEASE_GOARCH)' > release/TARGET
	cp -R new/dist/. release/member/
	cp -R new-back/dist/. release/admin/
	cp -R deploy/. release/deploy/
	cp scripts/production-config-check.sh scripts/production-readiness.sh scripts/production-deploy.sh scripts/production-rollback.sh scripts/postgres-backup.sh scripts/release-integrity.sh release/scripts/
	cp scripts/lib/backend-env.sh scripts/lib/safe-integer.sh scripts/lib/maintenance-edge.sh release/scripts/lib/
	cp PRODUCTION_OPERATIONS.md release/
	bash scripts/release-integrity.sh generate release
	@{ command -v sha256sum >/dev/null 2>&1 && sha256sum release/SHA256SUMS || shasum -a 256 release/SHA256SUMS; } | awk '{print "可信清单摘要（部署时使用 EXPECTED_MANIFEST_SHA256）：" $$1}'

production-check:
	bash scripts/production-readiness.sh

production-config-check:
	bash scripts/production-config-check.sh

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
