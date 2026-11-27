"""Fake downstream services for the durable-workflow labs (stdlib only, runs on the VM).

  python3 svc.py PORT CONFIG LOG

Every request is appended to LOG as one JSON line: {"t", "method", "path", "key", "body",
"status", "effect"}. "effect" is true when the call changed something (a charge, a booking, a
step done): the checks count effects to find work done twice. CONFIG (JSON, re-read on every
request, so a check can change behaviour mid-run) holds per-route behaviour, keyed by the first
path segment:

  {"charge": {"fail_first": 2,      first N calls answer 503
              "hang_first": 1,      first N calls sleep "hang" seconds before answering
              "hang": 30,
              "slow_first": 1,      first N calls apply the effect, then sleep "slow" seconds
              "slow": 8,            (the work is done but the answer arrives late)
              "decline_over": 500}, amounts above this answer 402 card_declined
   "book":   {"fail": ["car"]}}     these resources always answer 409 unavailable

Routes:
  GET  /balance/NAME            {"name", "balance"}
  POST /charge   {order, amount}            charge once per Idempotency-Key header (a repeated
                                            key returns the first answer, effect false)
  POST /refund   {order}                    refund an order's charge
  POST /book/WHAT {order}    -> {"booking"} WHAT = flight | hotel | car
  POST /cancel/WHAT {order, booking}
  POST /step/NAME {run, ...}                one step of a job (slow_first/hang apply too)
  POST /chunk {run, n}                      copy one chunk (takes "chunk_secs", default 1)
  POST /notify {to, text}                   send a message
  GET  /calls                               the log so far
"""
import json, os, sys, threading, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT, CONFIG, LOG = int(sys.argv[1]), sys.argv[2], sys.argv[3]
lock = threading.Lock()
seen = {}            # route -> number of calls so far
keys = {}            # Idempotency-Key -> (status, body)
BALANCES = {}


def config():
    try:
        with open(CONFIG) as f:
            return json.load(f)
    except Exception:
        return {}


def log(rec):
    rec["t"] = round(time.time(), 3)
    with lock, open(LOG, "a") as f:
        f.write(json.dumps(rec) + "\n")


class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def answer(self, status, body):
        data = json.dumps(body).encode()
        try:
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def do_GET(self):
        parts = self.path.strip("/").split("/")
        if parts[0] == "calls":
            try:
                with open(LOG) as f:
                    lines = [json.loads(l) for l in f if l.strip()]
            except FileNotFoundError:
                lines = []
            return self.answer(200, lines)
        if parts[0] == "balance" and len(parts) == 2:
            name = parts[1]
            bal = BALANCES.setdefault(name, (sum(map(ord, name)) * 37) % 9000 + 100)
            log({"method": "GET", "path": self.path, "status": 200, "effect": False})
            return self.answer(200, {"name": name, "balance": bal})
        self.answer(404, {"error": "not_found"})

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        try:
            body = json.loads(self.rfile.read(n) or b"{}")
        except ValueError:
            body = {}
        parts = self.path.strip("/").split("/")
        route = parts[0]
        key = self.headers.get("Idempotency-Key")
        cfg = config().get(route, {})
        with lock:
            seen[route] = seen.get(route, 0) + 1
            nth = seen[route]
        rec = {"method": "POST", "path": self.path, "key": key, "body": body}

        if nth <= cfg.get("fail_first", 0):
            rec.update(status=503, effect=False)
            log(rec)
            return self.answer(503, {"error": "unavailable", "retry": True})
        if nth <= cfg.get("hang_first", 0):
            time.sleep(cfg.get("hang", 30))
        if key and route in ("charge", "refund") and key in keys:
            status, out = keys[key]
            rec.update(status=status, effect=False, replay=True)
            log(rec)
            return self.answer(status, out)

        status, out, effect = 200, {}, True
        if route == "charge":
            if body.get("amount", 0) > cfg.get("decline_over", 10**9):
                status, out, effect = 402, {"error": "card_declined"}, False
            else:
                out = {"charge": "ch_" + uuid.uuid4().hex[:10], "order": body.get("order"), "amount": body.get("amount")}
        elif route == "refund":
            out = {"refunded": body.get("order")}
        elif route == "book" and len(parts) == 2:
            if parts[1] in cfg.get("fail", []):
                status, out, effect = 409, {"error": "unavailable", "what": parts[1]}, False
            else:
                out = {"booking": f"{parts[1]}-" + uuid.uuid4().hex[:8]}
        elif route == "cancel" and len(parts) == 2:
            out = {"cancelled": body.get("booking")}
        elif route == "step" and len(parts) == 2:
            out = {"done": parts[1]}
        elif route == "chunk":
            time.sleep(cfg.get("chunk_secs", 1))
            out = {"copied": body.get("n")}
        elif route == "notify":
            out = {"sent": True}
        else:
            status, out, effect = 404, {"error": "not_found"}, False

        if key and route in ("charge", "refund") and status == 200:
            keys[key] = (status, out)
        rec.update(status=status, effect=effect, out=out)
        log(rec)
        if effect and nth <= cfg.get("slow_first", 0):
            time.sleep(cfg.get("slow", 8))
        self.answer(status, out)


ThreadingHTTPServer.daemon_threads = True
ThreadingHTTPServer(("0.0.0.0", PORT), H).serve_forever()
