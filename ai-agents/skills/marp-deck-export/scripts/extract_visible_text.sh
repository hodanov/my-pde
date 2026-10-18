#!/usr/bin/env bash
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: extract_visible_text.sh <deck.md>" >&2
	exit 2
fi

deck="$1"
if [ ! -f "$deck" ]; then
	echo "error: deck not found: $deck" >&2
	exit 1
fi

perl -0777 -ne '
	s/\A---\n.*?\n---\n//s;
	s/<!--.*?-->//gs;
	my @slides = split /^---$/m;
	for my $i (0 .. $#slides) {
		(my $text = $slides[$i]) =~ s/\A\s+|\s+\z//g;
		$text =~ s/\n{3,}/\n\n/g;
		print "=== slide ", $i + 1, " ===\n", $text, "\n\n";
	}
' "$deck"
