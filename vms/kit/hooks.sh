#!/bin/bash
# Run (then forget) cleanup hooks registered by the previous exercise's setup.
source /opt/kubelab/lib.sh
D=$KL_STATE/cleanup.d
[ -d "$D" ] || exit 0
for f in $(ls "$D" 2>/dev/null | sort); do
  # run from the file, not from the command line: a hook's `pkill -f PATTERN` would
  # otherwise match (and kill) the shell running it, skipping the rest of the hook
  bash -c 'source /opt/kubelab/lib.sh; source "$1"' kl-hook "$D/$f" >/dev/null 2>&1 || true
  rm -f "$D/$f"
done
exit 0
