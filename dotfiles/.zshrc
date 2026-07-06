####################
# Prompt settings
####################
HOST_NAME=`hostname`
USER_NAME=`whoami`

typeset -g CMD_RAN=0

preexec() {
  TIMER=$EPOCHREALTIME
  CMD_RAN=1
}

# Simple
# PS1="%K{cyan}%F{black} $USER_NAME@$HOST_NAME %f%k%F{cyan}%K{white}%k%f%F{black}%K{white} %d %k%f "

# Rich
# autoload -Uz vcs_info
# zstyle ':vcs_info:git:*' formats ' %b'
# precmd() {
#   vcs_info
#   PS1="%K{cyan}%F{black} %n@%m %f%k\
# %F{cyan}%K{white}%F{black} %~ %f%k\
# %F{white} %F{46}${vcs_info_msg_0_}%f
# %(?.%F{cyan}.%F{yellow})λ%f "
# }

# Cyberpunk
# autoload -Uz vcs_info
# zstyle ':vcs_info:git:*' formats ' %b'
# precmd() {
#   vcs_info
#   PS1="%K{#0f0f1a}%F{#00eaff} %n@%m %f%k\
# %F{#0f0f1a}%K{#1a1a2e}%F{#ff2bd6} %~ %f%k\
# %F{#1a1a2e} %F{#00eaff}${vcs_info_msg_0_}%f
# %(?.%F{#00ff9c}.%F{#ff9f1c})λ%f "
# }

# Cyberpunk pastel
autoload -Uz vcs_info
zstyle ':vcs_info:git:*' formats '%F{#89dceb} %b%f'
precmd() {
  if (( CMD_RAN )); then
    print ""
  fi
  CMD_RAN=0
  vcs_info
  PS1="%K{#1e1e2e}%F{#9399b2} %n@%m %f%k\
%F{#1e1e2e}%K{#2a2a3c}%F{#cba6f7} %~ %f%k\
%F{#2a2a3c} %F{#94e2d5}${vcs_info_msg_0_}%f
%(?.%F{#a6e3a1}.%F{#f9e2af})λ%f "
}

RPROMPT="%F{#6c7086}%D{%m/%d %H:%M}%f"

export LC_ALL=en_US.UTF-8

####################
# Alias
####################
alias python="python3"
alias pip="pip3"
# alias nvim='docker container exec -it -w ${HOME}/workspace nvim-dev bash --login'
# bash --login -c "nvim \"\$@\"; exec bash --login"
nvim() {
  local host_pwd=$(pwd)
  docker container exec -it \
    -w "$host_pwd" \
    nvim-dev \
    bash --login -c "nvim \"\$@\""
}

####################
# Go path
####################
export PATH="$PATH:$(go env GOPATH)/bin"

####################
# Add Docker Desktop for Mac (docker)
####################
export PATH="$PATH:/Applications/Docker.app/Contents/Resources/bin/"

####################
# Load git and docker completion
####################
zstyle ':completion:*:*:git:*' script ~/.zsh/completion/git-completion.bash
fpath=(~/.zsh/completion $fpath)

# The following lines have been added by Docker Desktop to enable Docker CLI completions.
fpath=(/Users/hodanov/.docker/completions $fpath)
# End of Docker CLI completions

# 補完初期化は1回だけ。セキュリティ監査つきの dump 再生成は「24時間より
# 古いとき」だけ行い、それ以外はキャッシュ済み dump を -C で読んで監査を省略する。
autoload -Uz compinit
if [[ -n ${HOME}/.zcompdump(#qN.mh+24) ]]; then
  compinit -i
else
  compinit -C
fi

####################
# anyenv
####################
eval "$(anyenv init -)"

####################
# cursor cli
####################
export PATH="$HOME/.local/bin:$PATH"

####################
# curl
####################
export PATH="/usr/local/opt/curl/bin:$PATH"

####################
# ghq & peco
####################
# alias repo='cd $(ghq list --full-path --exact | peco)'

####################
# AI CLI
####################
alias al="claude"
export VISUAL=nvim
export EDITOR=nvim
export BLOG_IDEA_DRAFT_EXPORT_DIR="$HOME/workspace/hodalog-hugo/docs/idea"
# codex
eval "$(codex completion zsh)"

####################
# mise
####################
eval "$(mise activate zsh)"
