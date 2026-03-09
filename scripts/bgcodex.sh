#!/bin/bash

if [ -z "$1" ] || [ -z "$2" ]; then
    echo "Usage: $0 \"pane title\" \"your_prompt_here\" \"working directory\""
    exit 1
fi

PANE_TITLE="$1"
PROMPT="$2"
WORKING_DIR="${3:-$(pwd)}"

tmux set -g pane-border-status top 2>/dev/null

# Set the caller's pane title to "manager"
tmux select-pane -t "$TMUX_PANE" -T "manager"

if [ "${AGENT:-codex}" = "claude" ]; then
    RUN_CMD="claude \"$PROMPT\"; exec bash"
else
    RUN_CMD="codex \"$PROMPT\"; exec bash"
fi

# Split targeting the caller's pane so it always lands in the right window
PANE_ID=$(tmux split-window -d -h -t "$TMUX_PANE" -c "$WORKING_DIR" -P -F '#{pane_id}' "$RUN_CMD")
# Prevent the spawned process from overwriting the pane title
tmux set-option -t "$PANE_ID" allow-rename off
tmux select-pane -t "$PANE_ID" -T "$PANE_TITLE"
