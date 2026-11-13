#!/bin/sh
# Renders the tray PNGs from the SVG sources. The tray libraries take PNG only,
# so the SVGs stay the source of truth and the PNGs are regenerated with this.
set -eu
cd "$(dirname "$0")"

# tint <src> <out> <front> <side>: the sources are drawn in #E6E8EA (front and
# windows) and #A5A8B0 (side faces)
tint() {
	sed -e "s/#E6E8EA/$3/g" -e "s/#A5A8B0/$4/g" "$1" | rsvg-convert -w 32 -h 32 -o "$2"
}

# mono <src> <out>: a template icon is recoloured from its alpha alone, so the
# side faces become 70% of the one colour instead of a colour of their own
mono() {
	sed -e 's/fill="#A5A8B0" fill-opacity="1"/fill="#E6E8EA" fill-opacity="0.7"/g' -e 's/#E6E8EA/#FFFFFF/g' "$1" | rsvg-convert -w 32 -h 32 -o "$2"
}

tint running.svg icon-white.png '#E6E8EA' '#A5A8B0'
tint running.svg icon-dark.png '#1E1E1E' '#6B6E76'
tint running.svg icon-green.png '#28AA5A' '#1E7A42'
tint stopped.svg icon-off-white.png '#E6E8EA' '#A5A8B0'
tint stopped.svg icon-off-dark.png '#1E1E1E' '#6B6E76'
tint stopped.svg icon-off-red.png '#D23232' '#962424'
mono running.svg icon-mono.png
mono stopped.svg icon-mono-off.png
