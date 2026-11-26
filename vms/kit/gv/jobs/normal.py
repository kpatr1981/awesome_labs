# A well-behaved job: sum the numbers in /job/input.txt, using /tmp as scratch space.
import json, os

nums = [int(x) for x in open("/job/input.txt").read().split()]
scratch = "/tmp/kl-scratch.bin"
with open(scratch, "wb") as f:
    f.write(os.urandom(2 << 20))
tmp_ok = os.path.getsize(scratch) == 2 << 20
os.remove(scratch)
status = dict(l.split(":\t", 1) for l in open("/proc/self/status").read().splitlines() if ":\t" in l)
json.dump({
    "sum": sum(nums),
    "kernel": os.uname().release,
    "uid": os.getuid(), "euid": os.geteuid(),
    "cap_eff": status.get("CapEff", "").strip(),
    "no_new_privs": status.get("NoNewPrivs", "").strip(),
    "job_id": os.environ.get("JOB_ID"),
    "env": sorted(os.environ),
    "tmp_ok": tmp_ok,
}, open("/out/result.json", "w"))
print("job done: sum", sum(nums))
