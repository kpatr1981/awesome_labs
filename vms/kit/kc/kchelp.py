"""Helpers for the identity labs' hidden checks (run by kc_py in kl-idp:1)."""
import base64, hashlib, hmac, json, time

import requests

KC = "http://127.0.0.1:8080"


def token(realm, client, secret=None, user=None, pw=None, full=False, **extra):
    """An access token from the realm's token endpoint: client credentials, or the password grant."""
    d = {"client_id": client, **extra}
    if secret:
        d["client_secret"] = secret
    if user:
        d.update(grant_type="password", username=user, password=pw)
    else:
        d["grant_type"] = "client_credentials"
    r = requests.post(f"{KC}/realms/{realm}/protocol/openid-connect/token", data=d, timeout=15).json()
    return r if full else r.get("access_token")


def b64u(b):
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode()


def unb64u(s):
    return base64.urlsafe_b64decode(s + "=" * (-len(s) % 4))


def claims(tok):
    return json.loads(unb64u(tok.split(".")[1]))


def header(tok):
    return json.loads(unb64u(tok.split(".")[0]))


def forge(payload, alg="none", key=b"", kid=None):
    """A token with these claims, unsigned (alg none) or signed with HMAC-SHA256 and KEY."""
    h = {"alg": alg, "typ": "JWT"}
    if kid:
        h["kid"] = kid
    signing = b64u(json.dumps(h).encode()) + "." + b64u(json.dumps(payload).encode())
    if alg == "none":
        return signing + "."
    return signing + "." + b64u(hmac.new(key, signing.encode(), hashlib.sha256).digest())


def PUBKEY(realm):
    """The realm's RSA public key as PEM bytes (what an alg-confusion attacker would HMAC with)."""
    der = requests.get(f"{KC}/realms/{realm}", timeout=10).json()["public_key"]
    lines = [der[i:i + 64] for i in range(0, len(der), 64)]
    return ("-----BEGIN PUBLIC KEY-----\n" + "\n".join(lines) + "\n-----END PUBLIC KEY-----\n").encode()


def call(method, url, tok=None, scheme="Bearer", **kw):
    """One HTTP call with an Authorization header; returns status, body and WWW-Authenticate."""
    headers = kw.pop("headers", {})
    if tok is not None:
        headers["Authorization"] = f"{scheme} {tok}" if scheme else tok
    try:
        r = requests.request(method, url, headers=headers, timeout=15, **kw)
    except Exception as e:
        return {"status": 0, "body": f"{type(e).__name__}: {e}", "www": ""}
    try:
        body = r.json()
    except ValueError:
        body = r.text[:500]
    return {"status": r.status_code, "body": body, "www": r.headers.get("WWW-Authenticate", "")}
