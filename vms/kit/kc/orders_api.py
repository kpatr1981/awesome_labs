"""The orders API (reference version): accepts only tokens from realm shop meant for orders-api.

  GET  /orders   needs realm role orders:read     -> 200 {"orders": [...], "user": ...}
  POST /orders   needs realm role orders:write    -> 201 {"created": ..., "user": ...}
  No or bad token -> 401 with WWW-Authenticate; a good token without the role -> 403.
Every request is logged as one JSON line to $API_LOG (default /log/api.jsonl).
Environment: PORT (5000), ISSUER, AUDIENCE (orders-api), READ_ROLE (orders:read), WRITE_ROLE (orders:write).
"""
import json, os, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import jwt

ISSUER = os.environ.get("ISSUER", "http://127.0.0.1:8080/realms/shop")
AUDIENCE = os.environ.get("AUDIENCE", "orders-api")
PORT = int(os.environ.get("PORT", "5000"))
LOG = os.environ.get("API_LOG", "/log/api.jsonl")
READ_ROLE = os.environ.get("READ_ROLE", "orders:read")
WRITE_ROLE = os.environ.get("WRITE_ROLE", "orders:write")
jwks = jwt.PyJWKClient(ISSUER + "/protocol/openid-connect/certs", cache_keys=True)
ORDERS = [{"id": 1, "item": "keyboard"}, {"id": 2, "item": "monitor"}]


def verify(auth):
    scheme, _, tok = auth.partition(" ")
    if scheme.lower() != "bearer" or not tok:
        raise ValueError("missing bearer token")
    key = jwks.get_signing_key_from_jwt(tok).key
    return jwt.decode(tok, key, algorithms=["RS256"], audience=AUDIENCE, issuer=ISSUER,
                      options={"require": ["exp", "iat", "iss", "aud"]})


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def reply(self, status, body, claims=None, extra=None):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        for k, v in (extra or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(data)
        c = claims or {}
        try:
            with open(LOG, "a") as f:
                f.write(json.dumps({"t": round(time.time(), 3), "method": self.command, "path": self.path, "status": status,
                                    "azp": c.get("azp"), "user": c.get("preferred_username"), "jti": c.get("jti")}) + "\n")
        except OSError:
            pass

    def handle_orders(self, role):
        if self.path != "/orders":
            return self.reply(404, {"error": "not_found"})
        try:
            claims = verify(self.headers.get("Authorization", ""))
        except Exception as e:
            return self.reply(401, {"error": "invalid_token", "detail": str(e)},
                              extra={"WWW-Authenticate": 'Bearer realm="orders", error="invalid_token"'})
        if role not in claims.get("realm_access", {}).get("roles", []):
            return self.reply(403, {"error": "insufficient_scope", "needs": role}, claims,
                              {"WWW-Authenticate": f'Bearer realm="orders", error="insufficient_scope", scope="{role}"'})
        user = claims.get("preferred_username")
        if self.command == "GET":
            return self.reply(200, {"orders": ORDERS, "user": user}, claims)
        return self.reply(201, {"created": len(ORDERS) + 1, "user": user}, claims)

    def do_GET(self):
        if self.path == "/":
            return self.reply(200, {"ok": True})
        self.handle_orders(READ_ROLE)

    def do_POST(self):
        self.handle_orders(WRITE_ROLE)


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
