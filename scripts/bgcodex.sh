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
printf '\033]2;manager\033\\'

if [ "${AGENT:-codex}" = "claude" ]; then
    RUN_CMD="echo -ne '\033]2;${PANE_TITLE}\033\\'; claude \"$PROMPT\"; exec bash"
else
    RUN_CMD="echo -ne '\033]2;${PANE_TITLE}\033\\'; codex \"$PROMPT\"; exec bash"
fi

tmux split-window -d -h -c "$WORKING_DIR" "$RUN_CMD"
