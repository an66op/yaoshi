.PHONY: dev health smoke verify

dev:
	bash scripts/local-dev.sh

health:
	bash scripts/local-health.sh

smoke:
	bash scripts/local-smoke.sh

verify:
	cd backend && go test ./...
	cd new && npm run lint && npm run build
	cd new-back && npm run lint && npm run build
