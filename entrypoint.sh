#!/bin/sh

set -e
PUID=${PUID:-1000}
PGID=${PGID:-1000}
groupmod -o -g "$PGID" node
usermod -o -u "$PUID" node
exec su-exec node "$@"
