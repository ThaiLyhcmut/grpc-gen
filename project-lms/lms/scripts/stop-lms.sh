#!/bin/bash
# Stop everything started by run-lms.sh (reads run/pids).
set -uo pipefail
cd "$(dirname "$0")/.."   # -> project root (lms/)

if [ ! -f run/pids ]; then
  echo "nothing to stop (run/pids missing)"
  exit 0
fi

while read -r pid name; do
  [ -z "${pid:-}" ] && continue
  if kill "$pid" 2>/dev/null; then
    echo "stopped $name (pid $pid)"
  else
    echo "$name (pid $pid) already gone"
  fi
done < run/pids

rm -f run/pids
echo "done."
