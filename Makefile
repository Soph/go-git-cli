WORKDIR = $(PWD)
GOCMD = go

# Build output
CLI_BIN = $(WORKDIR)/build/bin

# Upstream git (for test infrastructure)
GIT_CLI_VERSION ?= v2.47.0
GIT_DIST_PATH ?= $(WORKDIR)/.git-dist
GIT_REPOSITORY = http://github.com/git/git.git

.PHONY: build build-git-testdeps test-cli test-cli-verbose gen-skip-list classify-failures clean

build:
	mkdir -p $(CLI_BIN)
	$(GOCMD) build -o $(CLI_BIN)/git ./cmd/git/...
	$(CLI_BIN)/git install

# Clone and build upstream git into .git-dist/ (needed for test-lib.sh,
# test-tool helpers, etc.). Cached — only clones/builds once.
build-git-testdeps:
	@if [ -f $(GIT_DIST_PATH)/git ]; then \
		echo "using cached git build at $(GIT_DIST_PATH)"; \
	else \
		echo "Cloning git $(GIT_CLI_VERSION) into $(GIT_DIST_PATH)..."; \
		git clone $(GIT_REPOSITORY) -b $(GIT_CLI_VERSION) --depth 1 --single-branch $(GIT_DIST_PATH); \
		echo "Building git in $(GIT_DIST_PATH)..."; \
		cd $(GIT_DIST_PATH) && make -j$$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4) all; \
	fi

# Run git test file(s) against the go-git CLI.
# Usage:
#   make test-cli                                  # run all key suites
#   make test-cli T=t7004-tag.sh                   # run a specific test
#   make test-cli T="t0001-init.sh t7004-tag.sh"   # run multiple
#   make test-cli-verbose T=t0001-init.sh          # verbose single test
test-cli: build build-git-testdeps
	@bash test/run-tests.sh $(GIT_DIST_PATH) $(CLI_BIN) $(T)

test-cli-verbose: build build-git-testdeps
ifndef T
	$(error T is required. Usage: make test-cli-verbose T=t0001-init.sh)
endif
	@bash test/run-tests.sh $(GIT_DIST_PATH) $(CLI_BIN) --verbose $(T)

# Re-generate the skip list by detecting which failures hit unimplemented commands.
gen-skip-list: build build-git-testdeps
	bash test/gen-skip-list.sh $(GIT_DIST_PATH) $(CLI_BIN)

# Classify test failures into categories (CLI shim vs go-git library bugs).
classify-failures: build build-git-testdeps
	@bash test/classify-failures.sh $(GIT_DIST_PATH) $(CLI_BIN) $(T)

clean:
	rm -rf $(GIT_DIST_PATH) build/
