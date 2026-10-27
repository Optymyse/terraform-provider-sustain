# The provider's steps (README.md). `make check` before every commit; `make acceptance` against a local preview.

OPENAPI_GENERATOR := github.com/hashicorp/terraform-plugin-codegen-openapi/cmd/tfplugingen-openapi@v0.3.0
DOCS_GENERATOR := github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0

.PHONY: generate check test acceptance docs

# Each resource's schema from api/sustain.json, Sustain's description: its copy, HashiCorp's specification of it, and
# the Go the provider runs (README.md, "How it's made").
generate:
	node tools/derive.mjs
	GOFLAGS=-mod=mod go run $(OPENAPI_GENERATOR) generate --config api/generator.yml --output api/provider-spec.json api/provider.json
	node tools/schemas.mjs

# Everything checked that needs no workspace: the tools' tests, what's generated and the docs as they'd be written now,
# Go's format, vet and tests.
check:
	node --test tools/derive.test.mjs tools/schemas.test.mjs
	$(MAKE) generate docs
	git diff --exit-code -- api internal/provider/resources_gen.go docs
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test ./...

test:
	go test ./...

# Each resource made, changed, imported and deleted by Terraform, in a writable local preview of Sustain's, which
# Sustain's repository starts: `pnpm ui:preview --port 4188 --plan enterprise --writable`. Never a real workspace.
# Terraform is the one TF_ACC_TERRAFORM_PATH names, or its latest, downloaded for the run.
acceptance:
	SUSTAIN_URL=$${SUSTAIN_URL:-http://acme.localhost:4188} SUSTAIN_TOKEN=$${SUSTAIN_TOKEN:-preview} TF_ACC=1 \
		go test ./internal/provider -run '^TestAcc' -count=1 -timeout 30m -v

# docs/, from each schema's descriptions and examples/.
docs:
	GOFLAGS=-mod=mod go run $(DOCS_GENERATOR) generate --provider-name sustain
