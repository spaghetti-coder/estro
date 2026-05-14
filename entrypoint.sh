#!/bin/sh

PUID="${PUID:-0}"
PGID="${PGID:-0}"

groupmod -o -g "${PGID}" app >/dev/null || exit
usermod -o -u "${PUID}" app >/dev/null  || exit

exec su-exec app "$@"
