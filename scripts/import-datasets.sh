#!/bin/sh
set -eu
DIR="$(cd "$(dirname "$0")/../database" && pwd)"
exec sh "$DIR/import.sh"
