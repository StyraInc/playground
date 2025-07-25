default: rego-playground

VERSION := $(shell ./scripts/get-version.sh)
OPA_VERSION := $(shell ./scripts/get-opa-version.sh)
OPA_RELEASE_VERSION := $(shell ./scripts/get-opa-release-version.sh $(OPA_VERSION))
USER_ID := $(shell id -u $(USER))
GROUP_ID := $(shell id -g $(USER))
NOW := $(shell date +%Y%m%d_%H%M%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DIR := $(shell echo `pwd`/build)
CONTAINER_TARGETS = rego-playground-container playground-gateway-container
PACKAGES := $(shell go list ./.../ | grep -v 'vendor')

GO := GO111MODULE=on GOFLAGS=-mod=vendor go

export TIMESTAMP ?= $(NOW)
export MAKE_DATE ?= $(NOW)
export DOCKER_REGISTRY ?= 547414210802.dkr.ecr.us-east-1.amazonaws.com
export DOCKER_REGISTRY_REPLICAS ?=
export UID ?= $(USER_ID)
export GID ?= $(GROUP_ID)

BUILD_FLAGS := -ldflags "-X github.com/open-policy-agent/rego-playground/version.Vcs=$(GIT_COMMIT) \
	-X github.com/open-policy-agent/rego-playground/version.Version=$(VERSION) \
	-X github.com/open-policy-agent/rego-playground/version.OPAVersion=$(OPA_VERSION) \
	-X github.com/open-policy-agent/rego-playground/version.OPAReleaseVersion=$(OPA_RELEASE_VERSION)"

.PHONY: all
all: clean test ui-deps ui-prod rego-playground

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)

.PHONY: rego-playground
rego-playground:
	mkdir -p $(BUILD_DIR)
	$(GO) build $(BUILD_FLAGS) -o $(BUILD_DIR)/rego-playground ./cmd/rego-playground/...

.PHONY: rego-playground-container
rego-playground-container:
	docker build -t "openpolicyagent/rego-playground" -t "openpolicyagent/rego-playground:${MAKE_DATE}" \
		--force-rm -f Dockerfile .

.PHONY: all-containers
all-containers: rego-playground-container

.PHONY: run-rego-playground-dev
run-rego-playground-dev:
	$(GO) run $(BUILD_FLAGS) ./cmd/rego-playground/main.go \
		--addr 127.0.0.1 \
		--no-persist \
		--external-url http://localhost:8181 \
		--ui-content-root ./build/ui \
		--verbose \
		--log-format json-pretty

.PHONY: update-opa
update-opa:
	@./scripts/update-opa-version.sh $(TAG)

.PHONY: test
test:
	$(GO) test $(BUILD_FLAGS) $(PACKAGES)

.PHONY: check
check: check-fmt check-vet check-lint

.PHONY: check-fmt
check-fmt:
	./scripts/check-fmt.sh

.PHONY: check-vet
check-vet:
	./scripts/check-vet.sh

.PHONY: check-lint
check-lint:
	./scripts/check-lint.sh

.PHONY: ui-clean
ui-clean:
	rm -rf ./build/ui

.PHONY: ui-deps
ui-deps:
	cd ui && npm install

.PHONY: policy-catalog-test
policy-catalog-test:
	opa test ./policy-catalog

.PHONY: ui-policy-catalog
ui-policy-catalog:
	$(GO) run ./cmd/policy-catalog-ui-js/main.go \
		--source policy-catalog \
		--destination ui/src/policies.js

.PHONY: ui-prod
ui-prod: ui-clean ui-policy-catalog
	cd ui && NODE_OPTIONS=--openssl-legacy-provider npx webpack --config ./webpack.prod.config.js

.PHONY: ui-dev
ui-dev: ui-clean ui-policy-catalog
	cd ui && npx webpack --config ./webpack.dev.config.js

.PHONY: ui-dev-watch
ui-dev-watch: ui-clean ui-policy-catalog
	cd ui && npx webpack --config ./webpack.dev.config.js --watch

.PHONY: ui-lint
ui-lint: ui-deps
		cd ui && npx eslint src/playground.js

.PHONY: ui-lint-fix
ui-lint-fix:
		cd ui && npx eslint src/playground.js --fix

.PHONY: ui-test
ui-test:
		cd ui && npm test