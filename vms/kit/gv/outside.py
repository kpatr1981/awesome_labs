"""The world outside the sandbox, for the Defensive track's sandboxing labs.

  outside.py LOG STATE ROLE=ADDR:PORT...

Roles: mirror (the package mirror the jobs may use), exfil (an attacker's server),
meta (a cloud-style metadata service with credentials). Every request is logged
to LOG as "ROLE METHOD PATH LENGTH"; STATE holds this run's tokens.
"""
import http.server, json, sys, threading

log_path, state_path = sys.argv[1], sys.argv[2]
state = json.load(open(state_path))
lock = threading.Lock()


def handler(role):
    class H(http.server.BaseHTTPRequestHandler):
        def _reply(self):
            n = int(self.headers.get("Content-Length") or 0)
            if n:
                self.rfile.read(min(n, 1 << 20))
            with lock, open(log_path, "a") as f:
                f.write(f"{role} {self.command} {self.path} {n}\n")
            if role == "mirror":
                if self.path.rstrip("/") == "/simple/kl-tools":
                    body = '<a href="/packages/kl-tools-1.4.2.txt">kl-tools-1.4.2</a>\n'
                elif self.path == "/packages/kl-tools-1.4.2.txt":
                    body = f"kl-tools 1.4.2 {state['mirror']}\n"
                else:
                    body = "kl package mirror\n"
            elif role == "meta":
                body = json.dumps({"AccessKeyId": "AKIAKLMETA", "SecretAccessKey": state["meta"]}) + "\n"
            else:
                body = "ok\n"
            b = body.encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(b)))
            self.end_headers()
            self.wfile.write(b)

        do_GET = do_POST = do_PUT = _reply

        def log_message(self, *a):
            pass

    return H


servers = []
for spec in sys.argv[3:]:
    role, addr = spec.split("=")
    host, port = addr.rsplit(":", 1)
    s = http.server.ThreadingHTTPServer((host, int(port)), handler(role))
    servers.append(s)
    threading.Thread(target=s.serve_forever, daemon=True).start()
threading.Event().wait()
