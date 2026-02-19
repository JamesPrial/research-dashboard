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

    # Pre-create directories the app needs (Unraid FUSE may block chown on mount points).
    mkdir -p /research/.claude/agents
    chown -R researcher:researcher /research/.claude

    # --- Claude CLI headless auth setup ---
    # The Claude CLI has interactive gates (onboarding, API key approval) that
    # block non-interactive (-p) usage. Pre-create config files to bypass them.
    # See: https://github.com/anthropics/claude-code/issues/551
    RESOLVED_KEY="${MAX_API_KEY:-$ANTHROPIC_API_KEY}"
    CLAUDE_HOME="/home/researcher"

    if [ -n "$RESOLVED_KEY" ]; then
        # 1. Mark onboarding as completed to skip interactive setup.
        cat > "$CLAUDE_HOME/.claude.json" <<ENDJSON
{"hasCompletedOnboarding": true}
ENDJSON

        # 2. Set up apiKeyHelper — the most reliable cross-version auth method.
        #    This shell script simply echoes the resolved API key.
        mkdir -p "$CLAUDE_HOME/.claude"
        cat > "$CLAUDE_HOME/.claude/get-api-key.sh" <<'ENDSCRIPT'
#!/bin/bash
echo "${MAX_API_KEY:-$ANTHROPIC_API_KEY}"
ENDSCRIPT
        chmod +x "$CLAUDE_HOME/.claude/get-api-key.sh"

        # 3. Point settings.json at the helper script.
        cat > "$CLAUDE_HOME/.claude/settings.json" <<ENDJSON
{"apiKeyHelper": "$CLAUDE_HOME/.claude/get-api-key.sh"}
ENDJSON

        chown -R researcher:researcher "$CLAUDE_HOME/.claude" "$CLAUDE_HOME/.claude.json"
    fi

    exec gosu researcher research-dashboard "$@"
fi

# If already running as non-root, just exec the binary directly.
exec research-dashboard "$@"
