# Hostile: try the system calls container escapes start with, and record what the kernel said.
import ctypes, json, os, platform

NR = {
    "aarch64": {"ptrace": 117, "unshare": 97, "mount": 40, "keyctl": 219, "add_key": 217, "bpf": 280, "userfaultfd": 282},
    "x86_64": {"ptrace": 101, "unshare": 272, "mount": 165, "keyctl": 250, "add_key": 248, "bpf": 321, "userfaultfd": 323},
}[platform.machine()]
ARGS = {
    "ptrace": (0, 0, 0, 0),                 # PTRACE_TRACEME
    "unshare": (0x10000000, 0, 0, 0),       # CLONE_NEWUSER
    "mount": (0, 0, 0, 0),
    "keyctl": (0, 0, 0, 0),
    "add_key": (0, 0, 0, 0),
    "bpf": (0, 0, 0, 0),
    "userfaultfd": (0, 0, 0, 0),
}
libc = ctypes.CDLL(None, use_errno=True)
r = {}
for name, nr in NR.items():
    ctypes.set_errno(0)
    rc = libc.syscall(nr, *[ctypes.c_long(a) for a in ARGS[name]])
    e = ctypes.get_errno()
    r[name] = "allowed" if rc >= 0 else ("EPERM" if e == 1 else os.strerror(e))
json.dump(r, open("/out/syscalls.json", "w"), indent=1)
print(json.dumps(r, indent=1))
