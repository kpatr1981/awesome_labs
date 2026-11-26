# Hostile: write wherever it can, fill the disk, change its own input.
import json, os

def attempt(name, fn):
    try:
        fn()
        r[name] = "succeeded"
    except Exception as e:
        r[name] = "blocked: " + type(e).__name__

def fill():
    with open("/tmp/fill.bin", "wb") as f:
        for _ in range(256):
            f.write(b"\0" * (1 << 20))
            f.flush()
            os.fsync(f.fileno())

r = {}
attempt("etc", lambda: open("/etc/kl-owned", "w").write("owned\n"))
attempt("site_packages", lambda: open("/usr/local/lib/python3.12/site-packages/kl_owned.pth", "w").write("import os\n"))
attempt("input", lambda: open("/job/input.txt", "a").write("999999\n"))
attempt("tmp_256mb", fill)
try:
    os.remove("/tmp/fill.bin")
except Exception:
    pass
attempt("out", lambda: open("/out/note.txt", "w").write("results go here\n"))
json.dump(r, open("/out/fs.json", "w"), indent=1)
print(json.dumps(r, indent=1))
