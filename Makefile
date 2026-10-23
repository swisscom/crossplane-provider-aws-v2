# ====================================================================================
# Setup Project
PROJECT_NAME := provider-aws-v2
PROJECT_REPO := github.com/swisscom/crossplane-$(PROJECT_NAME)

PLATFORMS ?= linux_amd64 linux_arm64
-include build/makelib/common.mk

# ====================================================================================
# Setup Output

-include build/makelib/output.mk

# ====================================================================================
# Setup Go

# Every go command run by make uses go.mod's own Go version, the same one CI
# sets up, downloading it on first use if the local Go differs. golangci-lint
# in particular is a prebuilt binary that cannot typecheck a standard library
# newer than the Go that built it, and GOTOOLCHAIN=auto only ever upgrades.
# Override with GOTOOLCHAIN=local to use the Go on your PATH instead.

GOTOOLCHAIN ?= go$(shell awk '$$1 == "go" { v = $$2; if (v ~ /^[0-9]+\.[0-9]+$$/) v = v ".0"; print v; exit }' go.mod)
export GOTOOLCHAIN

NPROCS ?= 1
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))
GO_STATIC_PACKAGES = $(GO_PROJECT)/cmd/provider
GO_LDFLAGS += -X $(GO_PROJECT)/internal/version.Version=$(VERSION)
GO_SUBDIRS += cmd internal apis
GO111MODULE = on
GOLANGCILINT_VERSION = 2.15.0
-include build/makelib/golang.mk

# ====================================================================================
# Setup Kubernetes tools

# The submodule defaults to a 1.x CLI, which drops spec.capabilities when it
# rebuilds package/crossplane.yaml. The package then installs and reports
# Healthy but reconciles nothing. Matches crossplane/apis/v2 in go.mod.
CROSSPLANE_CLI_VERSION ?= v2.3.4
-include build/makelib/k8s_tools.mk

# ====================================================================================
# Setup Images

IMAGES = provider-aws-v2
-include build/makelib/imagelight.mk

# ====================================================================================
# Setup XPKG

XPKG_REG_ORGS ?= ghcr.io/swisscom
# NOTE(hasheddan): skip promoting on xpkg.upbound.io as channel tags are
# inferred.
XPKG_REG_ORGS_NO_PROMOTE ?= ghcr.io/swisscom
XPKGS = provider-aws-v2
-include build/makelib/xpkg.mk

# NOTE(hasheddan): we force image building to happen prior to xpkg build so that
# we ensure image is present in daemon.
xpkg.build.provider-aws-v2: do.build.images

fallthrough: submodules
	@echo Initial setup complete. Running make again . . .
	@make

# integration tests
e2e.run: test-integration

# Run integration tests.
test-integration: $(KIND) $(KUBECTL) $(CROSSPLANE_CLI) $(HELM3)
	@$(INFO) running integration tests using kind $(KIND_VERSION)
	@KIND_NODE_IMAGE_TAG=${KIND_NODE_IMAGE_TAG} $(ROOT_DIR)/cluster/local/integration_tests.sh || $(FAIL)
	@$(OK) integration tests passed

# Update the submodules, such as the common build scripts.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

# NOTE(hasheddan): the build submodule currently overrides XDG_CACHE_HOME in
# order to force the Helm 3 to use the .work/helm directory. This causes Go on
# Linux machines to use that directory as the build cache as well. We should
# adjust this behavior in the build submodule because it is also causing Linux
# users to duplicate their build cache, but for now we just make it easier to
# identify its location in CI so that we cache between builds.
go.cachedir:
	@go env GOCACHE

go.mod.cachedir:
	@go env GOMODCACHE

# NOTE(hasheddan): we must ensure up is installed in tool cache prior to build
# as including the k8s_tools machinery prior to the xpkg machinery sets UP to
# point to tool cache.
build.init: $(CROSSPLANE_CLI)

# This is for running out-of-cluster locally, and is for convenience. Running
# this make target will print out the command which was used. For more control,
# try running the binary directly with different arguments.
run: go.build
	@$(INFO) Running Crossplane locally out-of-cluster . . .
	@# To see other arguments that can be provided, run the command with --help instead
	$(GO_OUT_DIR)/provider --debug

dev: $(KIND) $(KUBECTL)
	@$(INFO) Creating kind cluster
	@$(KIND) create cluster --name=$(PROJECT_NAME)-dev
	@$(KUBECTL) cluster-info --context kind-$(PROJECT_NAME)-dev
	@$(INFO) Installing provider-aws-v2 CRDs
	@$(KUBECTL) apply -R -f package/crds
	@$(INFO) Starting provider-aws-v2 controllers
	@$(GO) run cmd/provider/main.go --debug

dev-clean: $(KIND) $(KUBECTL)
	@$(INFO) Deleting kind cluster
	@$(KIND) delete cluster --name=$(PROJECT_NAME)-dev

# Generate the API types and controllers of AWS services, e.g.
# make services SERVICES="servicecatalog rds". Each service needs
# apis/<service>/generator-config.yaml. Run make generate afterwards.
services:
	@$(if $(SERVICES),,$(ERR) SERVICES is required, e.g. make services SERVICES=servicecatalog; exit 1)
	@for s in $(SERVICES); do \
		$(INFO) Generating $$s; \
		(cd codegen && $(GO) run ./cmd/generate --service $$s --output .. $(if $(CODEGEN_CACHE_DIR),--cache-dir $(abspath $(CODEGEN_CACHE_DIR)))) || exit 1; \
	done
	@$(OK) Generated $(SERVICES)

# Regenerate every service that has a generator config. CI runs this before
# check-diff, so a hand-edited or stale zz_ file fails the build.
ALL_SERVICES := $(patsubst apis/%/generator-config.yaml,%,$(wildcard apis/*/generator-config.yaml))
services.all:
	@$(MAKE) services SERVICES="$(ALL_SERVICES)"

# Unit tests of the code generator, which is a separate Go module.
codegen.test:
	@$(INFO) Testing codegen
	@cd codegen && $(GO) test ./...
	@$(OK) Testing codegen

# golangci-lint of the code generator; ./... of the root module stops at
# codegen/go.mod. Uses the root .golangci.yml.
codegen.lint: $(GOLANGCILINT)
	@$(INFO) golangci-lint codegen
	@cd codegen && $(GOLANGCILINT) run ./... || $(FAIL)
	@$(OK) golangci-lint codegen

# make test and make lint cover both modules.
test.run: codegen.test
lint.run: codegen.lint

# Fails while code from codegen/cmd/import still needs porting by hand.
check-import-todo:
	@$(INFO) Checking for unported import leftovers
	@./hack/check-import-todo.sh || $(FAIL)
	@$(OK) No unported import leftovers

# Both modules must build with the same Go, so a bump of only one fails CI.
GO_LINES = awk '$$1 == "go" || $$1 == "toolchain" { printf "%s%s %s", sep, $$1, $$2; sep = ", " }'
toolchain.check:
	@a=$$($(GO_LINES) go.mod); b=$$($(GO_LINES) codegen/go.mod); \
	echo "go.mod: $$a; codegen/go.mod: $$b"; \
	if [ -z "$$a" ] || [ "$$a" != "$$b" ]; then $(ERR) both go.mod files need the same go and toolchain lines; exit 1; fi
	@$(OK) Go versions match

modules.check: toolchain.check

# The govulncheck version is pinned; the vulnerability database is always the
# current one, so new advisories fail CI without a code change.
GOVULNCHECK_VERSION ?= v1.8.0
vulncheck:
	@$(INFO) govulncheck $(GOVULNCHECK_VERSION)
	@$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./... || $(FAIL)
	@cd codegen && $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./... || $(FAIL)
	@$(OK) govulncheck

# Scans the provider binary we ship, which also covers the Go it was built with.
vulncheck.binary:
	@$(INFO) govulncheck $(GO_OUT_DIR)/provider
	@$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) -mode=binary $(GO_OUT_DIR)/provider || $(FAIL)
	@$(OK) govulncheck binary

ACTIONLINT_VERSION ?= v1.7.12
ZIZMOR_VERSION ?= 1.30.1
# pipx is on the GitHub runners; use ZIZMOR="uvx zizmor==1.30.1" locally if you prefer uv.
ZIZMOR ?= pipx run zizmor==$(ZIZMOR_VERSION)
# Without a token zizmor skips the audits that need the GitHub API.
workflows.lint:
	@$(INFO) actionlint $(ACTIONLINT_VERSION), zizmor $(ZIZMOR_VERSION)
	@$(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION) || $(FAIL)
	@$(ZIZMOR) $(if $(GH_TOKEN),,--offline) --config .github/zizmor.yml .github/workflows || $(FAIL)
	@$(OK) workflows lint

.PHONY: submodules fallthrough test-integration run dev dev-clean services services.all codegen.test codegen.lint toolchain.check vulncheck vulncheck.binary workflows.lint check-import-todo

# ====================================================================================
# Special Targets

define CROSSPLANE_MAKE_HELP
Crossplane Targets:
    submodules            Update the submodules, such as the common build scripts.
    run                   Run crossplane locally, out-of-cluster. Useful for development.
    services              Generate AWS services, e.g. make services SERVICES=servicecatalog.
    services.all          Regenerate every service with an apis/<svc>/generator-config.yaml.
    toolchain.check       Check that both go.mod files have the same go and toolchain lines.
    vulncheck             Run govulncheck on both Go modules.
    vulncheck.binary      Run govulncheck on the built provider binary.
    workflows.lint        Lint the GitHub workflows with actionlint and zizmor.
    codegen.test          Run the unit tests of the code generator.
    codegen.lint          Run golangci-lint on the code generator.
    check-import-todo     Fail if *_importtodo.go files or TODO(import) markers are left.

endef
# The reason CROSSPLANE_MAKE_HELP is used instead of CROSSPLANE_HELP is because the crossplane
# binary will try to use CROSSPLANE_HELP if it is set, and this is for something different.
export CROSSPLANE_MAKE_HELP

crossplane.help:
	@echo "$$CROSSPLANE_MAKE_HELP"

help-special: crossplane.help

.PHONY: crossplane.help help-special
