#!/bin/bash
# Run (then forget) cleanup hooks registered by the previous exercise's setup.
source /opt/kubelab/lib.sh
D=$KL_STATE/cleanup.d
[ -d "$D" ] || exit 0
for f in $(ls "$D" 2>/dev/null | sort); do
  bash -c "source /opt/kubelab/lib.sh; $(cat "$D/$f")" >/dev/null 2>&1 || true
  rm -f "$D/$f"
done
exit 0
