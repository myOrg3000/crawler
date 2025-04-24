LOCAL_BIN := $(CURDIR)/bin

GOLANGCI_BIN := $(LOCAL_BIN)/golangci-lint
GOLANGCI_TAG=1.61.0

GO_TEST=$(LOCAL_BIN)/gotest
GO_TEST_ARGS="-race -v ./..."

# Выполнить полный цикл
all: lint test

# Устанавливает зависимости для использования
.PHONY: install-deps
install-deps:
	echo 'Installing dependencies...'
	tmp=$$(mktemp -d) && cd $$tmp && pwd && go mod init temp && \
	GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@v$(GOLANGCI_TAG) && \
	GOBIN=$(LOCAL_BIN) go install github.com/rakyll/gotest@v0.0.6 && \
	rm -fr $$tmp

# Запускает линтер по файлам
.PHONY: lint
lint: install-deps
	echo 'Running linter on files...'
	$(GOLANGCI_BIN) run \
	--config=.golangci.yaml \
	--sort-results \
	--max-issues-per-linter=0 \
	--max-same-issues=0

# Прогоняет все тесты
.PHONY: test
test: install-deps
	echo 'Running tests...'
	${GO_TEST} "${GO_TEST_ARGS}"

# Обновить репозиторий
.PHONY: update
update:
	@if [ -n "$(shell git status --untracked-files=no --porcelain)" ]; then \
		echo 'You have some changes. Please commit, checkout or stash them.'; \
		exit 1; \
	fi
	@current_branch=$$(git branch --show-current); \
	echo "Current branch: $$current_branch"; \
	git checkout main; \
	git pull; \
	for branch in $$(git branch | sed 's/^[* ]*//'); do \
		git checkout $$branch; \
		if ! git rev-parse --symbolic-full-name @{u} >/dev/null 2>&1; then \
			branch_exists=$$(git ls-remote --heads origin $$branch); \
			if [ -n "$$branch_exists" ]; then \
				echo "Upstream exists for $$branch. Setting upstream to origin/$$branch."; \
				git branch --set-upstream-to=origin/$$branch; \
			else \
				echo "Upstream not found for $$branch. Pushing and setting upstream to origin/$$branch."; \
				git push --set-upstream origin $$branch; \
				git branch --set-upstream-to=origin/$$branch; \
			fi; \
		fi; \
		git pull --rebase; \
		git push -f; \
		git rebase main; \
		git push -f; \
	done; \
	git checkout $$current_branch; \
	echo 'Successfully updated'
