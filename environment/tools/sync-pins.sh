#!/usr/bin/env bash
# Regenerate pinned artifacts from mise.toml (single source of truth):
#   - environment/tools/go/go-tools.txt (consumed by the Docker image build)
#   - ARG defaults in environment/docker/nvim.dockerfile
# Run via `mise run pins:sync`; CI verifies sync with `mise run pins:check`.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MISE_TOML="$ROOT/mise.toml"
GO_TOOLS="$ROOT/environment/tools/go/go-tools.txt"
DOCKERFILE="$ROOT/environment/docker/nvim.dockerfile"

# Extract the value of a `key = "value"` line. Pass quoted keys as-is
# (e.g. '"go:golang.org/x/tools/gopls"').
pin() {
	local value
	value=$(sed -n "s|^${1} = \"\(.*\)\"$|\1|p" "$MISE_TOML" | head -1)
	if [ -z "$value" ]; then
		echo "sync-pins: pin not found in mise.toml: ${1}" >&2
		exit 1
	fi
	printf '%s' "$value"
}

goimports=$(pin '"go:golang.org/x/tools/cmd/goimports"')
gopls=$(pin '"go:golang.org/x/tools/gopls"')
dlv=$(pin '"go:github.com/go-delve/delve/cmd/dlv"')
golangci_lint=$(pin 'golangci-lint')
langserver=$(pin '"go:github.com/nametake/golangci-lint-langserver"')
terraform_ls=$(pin 'terraform-ls')
tflint=$(pin 'tflint')

# NOTE: no comments or blank lines — the Dockerfile `go install`s every line.
cat >"$GO_TOOLS" <<EOF
golang.org/x/tools/cmd/...@v${goimports}
golang.org/x/tools/gopls@v${gopls}
github.com/go-delve/delve/cmd/dlv@v${dlv}
github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${golangci_lint}
github.com/nametake/golangci-lint-langserver@v${langserver}
github.com/hashicorp/terraform-ls@v${terraform_ls}
github.com/terraform-linters/tflint@v${tflint}
EOF
echo "Generated $GO_TOOLS"

go_version=$(pin 'go')
node_version=$(pin 'node')
neovim_version=$(pin 'NEOVIM_VERSION')
npm_version=$(pin 'NPM_VERSION')
rust_toolchain=$(pin 'RUST_TOOLCHAIN')
terraform_version=$(pin 'TERRAFORM_VERSION')

sed -i.bak -E \
	-e "s|^ARG GO_VERSION(=.*)?$|ARG GO_VERSION=${go_version}|" \
	-e "s|^ARG NODE_VERSION(=.*)?$|ARG NODE_VERSION=${node_version}|" \
	-e "s|^ARG NEOVIM_VERSION(=.*)?$|ARG NEOVIM_VERSION=${neovim_version}|" \
	-e "s|^ARG NPM_VERSION(=.*)?$|ARG NPM_VERSION=${npm_version}|" \
	-e "s|^ARG RUST_TOOLCHAIN(=.*)?$|ARG RUST_TOOLCHAIN=${rust_toolchain}|" \
	-e "s|^ARG TERRAFORM_VERSION(=.*)?$|ARG TERRAFORM_VERSION=${terraform_version}|" \
	"$DOCKERFILE"
rm -f "${DOCKERFILE}.bak"
echo "Synced ARG defaults in $DOCKERFILE"
