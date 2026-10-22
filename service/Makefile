.PHONY: setup check check-rust build test dev migrate-local bundle smoke bundle-production migrate-production deploy backup-production
WORKER_BUILD_VERSION := 0.8.7

setup:
	rustup target add wasm32-unknown-unknown
	@if [ "$$(.tools/bin/worker-build --version 2>/dev/null)" = "$(WORKER_BUILD_VERSION)" ]; then \
		printf 'worker-build %s is already installed\n' '$(WORKER_BUILD_VERSION)'; \
	else \
		cargo install worker-build --version '$(WORKER_BUILD_VERSION)' --locked --root .tools --force; \
	fi
	npm ci
check-rust:
	cargo fmt --check
	cargo clippy --locked --target wasm32-unknown-unknown -- -D warnings
	cargo test --locked
build:
	.tools/bin/worker-build --release --no-panic-recovery
check: check-rust test
# workerd/D1 is a JavaScript host; these tests call only the generated Rust Worker.
test: build
	npm exec -- vitest run
bundle:
	npm exec -- cf deploy --mode development --dry-run
dev:
	npm exec -- cf dev --mode development
migrate-local:
	# cf beta rejects the existing nil local database ID. Preserve local data.
	npm exec -- wrangler d1 migrations apply DB --local
smoke:
	go run ../devtools/service-smoke

bundle-production:
	npm exec -- cf deploy --mode production --dry-run
migrate-production:
	npm exec -- cf d1 migrations apply 43659423-7b12-42ac-8f1e-fc97f3ed0fea --mode production --dir migrations
deploy:
	npm exec -- cf deploy --mode production --secrets-file .env.production
backup-production:
	sh scripts/backup-production.sh
