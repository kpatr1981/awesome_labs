"""Calls to the shop's services on 127.0.0.1:9100 (standard library only).

    import shop
    shop.get("/balance/alice")                                   -> {"name": ..., "balance": ...}
    shop.post("/charge", {"order": "o-1", "amount": 40}, key="o-1-charge")

A non-2xx answer raises shop.ServiceError (.status is the HTTP code, .body the JSON answer).
`timeout` is the socket timeout in seconds (default 120).
"""
import json
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:9100"


class ServiceError(Exception):
    def __init__(self, status, body):
        self.status, self.body = status, body
        super().__init__(f"HTTP {status}: {body}")


def call(method, path, body=None, key=None, timeout=120):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    if key:
        req.add_header("Idempotency-Key", key)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        try:
            payload = json.loads(e.read() or b"{}")
        except ValueError:
            payload = {}
        raise ServiceError(e.code, payload) from None


def get(path, timeout=120):
    return call("GET", path, timeout=timeout)


def post(path, body, key=None, timeout=120):
    return call("POST", path, body, key, timeout)
