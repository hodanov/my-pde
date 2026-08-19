# goenv
export PATH="$PATH:/usr/local/go/bin"

GOROOT=$(go env GOROOT)
export PATH="$PATH:${GOROOT}/bin"

GOPATH=$(go env GOPATH)
export PATH="$PATH:${GOPATH}/bin"

# shell autocompletion about uv/uvx
eval "$(uv generate-shell-completion bash)"
eval "$(uvx --generate-shell-completion bash)"

# Shared prompt (also sourced automatically for non-login shells via .bashrc)
# shellcheck source=./.bashrc
[ -f ~/.bashrc ] && . ~/.bashrc
