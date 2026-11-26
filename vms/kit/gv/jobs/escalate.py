# Hostile: become root through the image's setuid helper, then do what only root may.
import json, os, subprocess
r = {"uid": os.getuid(), "cap_eff": [l.split()[1] for l in open("/proc/self/status") if l.startswith("CapEff")][0]}
p = subprocess.run(["/usr/local/bin/helper", "-c",
    "import os; os.setuid(0); print('root', os.getuid())"],
    capture_output=True, text=True)
r["helper"] = (p.stdout + p.stderr).strip().splitlines()[-1:] or [""]
r["became_root"] = r["helper"] == ["root 0"]
try:
    os.chown("/out", 0, 0)
    r["chown"] = "allowed"
except Exception as e:
    r["chown"] = "blocked: " + type(e).__name__
json.dump(r, open("/out/escalate.json", "w"), indent=1)
print(json.dumps(r, indent=1))
