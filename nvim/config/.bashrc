# Prompt ported from dotfiles/.zshrc's "Cyberpunk pastel" theme (Catppuccin Mocha).
# Sourced automatically by bash for interactive non-login shells (nvim's
# <Leader>-/<Leader>l term://bash), and via .bash_profile for login shells.
__prompt_git_branch() {
	local ref
	ref=$(git symbolic-ref --quiet --short HEAD 2>/dev/null) || ref=$(git rev-parse --short HEAD 2>/dev/null) || return
	__prompt_branch_segment=' \[\e[38;2;137;220;235m\] '"$ref"'\[\e[0m\]'
}

__set_prompt() {
	local exit_code=$?
	local arrow_color
	if [ "$exit_code" -eq 0 ]; then
		arrow_color='\[\e[38;2;166;227;161m\]'
	else
		arrow_color='\[\e[38;2;249;226;175m\]'
	fi

	__prompt_branch_segment=''
	__prompt_git_branch

	PS1='\[\e[48;2;30;30;46m\]\[\e[38;2;147;153;178m\] \u@my-nvim \[\e[0m\]'
	PS1+='\[\e[48;2;42;42;60m\]\[\e[38;2;203;166;247m\] \w \[\e[0m\]'
	PS1+="$__prompt_branch_segment"
	PS1+=$'\n'
	PS1+="${arrow_color}λ\[\e[0m\] "
}
PROMPT_COMMAND=__set_prompt
