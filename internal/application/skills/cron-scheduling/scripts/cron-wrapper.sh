#!/bin/bash
# cron-wrapper.sh — Run a command with logging, timing, and error alerting
# Usage: cron-wrapper.sh <job-name> <command> [args...]

set -euo pipefail

JOB_NAME="${1:?Usage: cron-wrapper.sh <job-name> <command> [args...]}"
shift
COMMAND=("$@")

LOG_DIR="/var/log/cron-jobs"
mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/$JOB_NAME.log"

log() { echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*" >> "$LOG_FILE"; }

log "START: ${COMMAND[*]}"
START_TIME=$(date +%s)

if "${COMMAND[@]}" >> "$LOG_FILE" 2>&1; then
    ELAPSED=$(( $(date +%s) - START_TIME ))
    log "SUCCESS (${ELAPSED}s)"
else
    EXIT_CODE=$?
    ELAPSED=$(( $(date +%s) - START_TIME ))
    log "FAILED with exit code $EXIT_CODE (${ELAPSED}s)"
    # Alert (customize as needed)
    echo "Cron job '$JOB_NAME' failed with exit $EXIT_CODE" | \
        mail -s "CRON FAIL: $JOB_NAME" admin@example.com 2>/dev/null || true
    exit $EXIT_CODE
fi
