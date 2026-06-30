BINARY := vmup

.PHONY: build run clean build-all docs-serve docs-build docs-deploy docs-clean

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY)-*

build-all:
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 .
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build -o $(BINARY)-windows-amd64.exe .

# --- Documentation (MkDocs Material via uv) ---
# uvx runs mkdocs in an ephemeral, cached environment, so no global install
# is required (the first run resolves and caches the dependencies).
MKDOCS := uvx --with mkdocs-material mkdocs

docs-serve:
	$(MKDOCS) serve

docs-build:
	$(MKDOCS) build --strict

docs-deploy:
	$(MKDOCS) gh-deploy --force

docs-clean:
	rm -rf site
