#!/bin/sh
# Builds the macOS app bundle and installs it into the Applications folder.
#
#   scripts/install-app.sh                 build, then install into /Applications
#   INSTALL_DIR=~/Applications scripts/install-app.sh
#
# /Applications is used when it is writable, else ~/Applications. A copy
# already installed there is replaced; quit the app first.
set -eu

if [ "$(uname)" != "Darwin" ]; then
	echo "install-app: macOS only" >&2
	exit 1
fi

cd "$(dirname "$0")/.."

name=$(sed -n 's/.*"name": *"\([^"]*\)".*/\1/p' mygo.json | head -n 1)
out=$(sed -n 's/.*"out": *"\([^"]*\)".*/\1/p' mygo.json | head -n 1)
out=${out:-build}

echo "Building $name.app…"
go tool mygo build --skip-dmg

app=$(find "$out" -maxdepth 4 -type d -name "$name.app" -print -quit)
if [ -z "$app" ]; then
	echo "install-app: no $name.app under $out/" >&2
	exit 1
fi

dest=${INSTALL_DIR:-/Applications}
if [ -z "${INSTALL_DIR:-}" ] && [ ! -w "$dest" ]; then
	dest="$HOME/Applications"
fi
mkdir -p "$dest"

if pgrep -xq "$name"; then
	echo "install-app: $name is running; quit it and run this again" >&2
	exit 1
fi

target="$dest/$name.app"
rm -rf "$target"
ditto "$app" "$target"
echo "Installed $target"
