#!/bin/bash

if [ -z "$1" ] || [ -z "$2" ]; then
    echo "Usage: $0 \"pane title\" \"your_prompt_here\" \"working directory\""
    exit 1
fi

WINDOW_NAME="codex-agents"
PANE_TITLE="$1"
PROMPT="$2"
WORKING_DIR="${3:-$(pwd)}"
LOCK_NAME="${WINDOW_NAME}-window-lock"

tmux set -g pane-border-status top 2>/dev/null

RUN_CMD="echo -ne '\033]2;${PANE_TITLE}\033\\'; codex \"$PROMPT\"; exec bash"

tmux wait-for -L "$LOCK_NAME"
cleanup() {
    tmux wait-for -U "$LOCK_NAME" 2>/dev/null
}
trap cleanup EXIT

if tmux has-session -t ":$WINDOW_NAME" 2>/dev/null; then
    tmux split-window -d -h -t ":$WINDOW_NAME" -c "$WORKING_DIR" "$RUN_CMD"
else
    tmux new-window -d -n "$WINDOW_NAME" -c "$WORKING_DIR" "$RUN_CMD"
fi
