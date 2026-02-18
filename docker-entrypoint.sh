#!/bin/bash
set -e

# Default PUID/PGID to the researcher user's current UID/GID.
# Unraid users should set PUID=99 PGID=100 (nobody:users).
PUID=${PUID:-$(id -u researcher)}
PGID=${PGID:-$(id -g researcher)}

# If running as root, handle user remapping and privilege drop.
if [ "$(id -u)" = "0" ]; then
    CURRENT_UID=$(id -u researcher)
    CURRENT_GID=$(id -g researcher)

    # Remap group if needed (-o allows non-unique GID).
    if [ "$PGID" != "$CURRENT_GID" ]; then
        groupmod -o -g "$PGID" researcher
    fi

    # Remap user if needed (-o allows non-unique UID).
    if [ "$PUID" != "$CURRENT_UID" ]; then
        usermod -o -u "$PUID" researcher
    fi

    # Fix ownership of critical directories.
    chown researcher:researcher /research
    # Home dir chown may partially fail if .claude is mounted read-only.
    chown -R researcher:researcher /home/researcher 2>/dev/null || true

    exec gosu researcher research-dashboard "$@"
fi

# If already running as non-root, just exec the binary directly.
exec research-dashboard "$@"
