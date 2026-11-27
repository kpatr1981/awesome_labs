"""A scripted browser login through Keycloak (requests only).

  python login.py URL USER PASSWORD     -> JSON on stdout:
    {"status": final HTTP status, "url": final URL, "body": start of the page,
     "login_page": whether Keycloak's login form was shown, "kc_error": Keycloak's error text,
     "first": {"status", "location"} of the very first request, "hops": the URLs visited}

It opens URL like a browser: it follows redirects and keeps cookies, pressing oauth2-proxy's
"Sign in with ..." button if that page comes up. If it lands on Keycloak's login form, it fills
in USER and PASSWORD, submits, and follows the redirects back. Browsers send Secure cookies to
http://127.0.0.1 (a "potentially trustworthy" origin); Python doesn't, so the Secure flag is
dropped from every cookie received.
"""
import html, json, re, sys
from urllib.parse import urljoin

import requests

url, user, pw = sys.argv[1], sys.argv[2], sys.argv[3]
s = requests.Session()
s.headers["User-Agent"] = "kl-login/1"
out = {"login_page": False, "kc_error": "", "hops": []}


def go(method, u, **kw):
    for _ in range(20):
        r = s.request(method, u, allow_redirects=False, timeout=15, **kw)
        for c in s.cookies:
            c.secure = False
        out["hops"].append(f"{r.status_code} {u[:120]}")
        loc = r.headers.get("Location")
        if r.status_code in (301, 302, 303, 307, 308) and loc:
            u, method, kw = urljoin(u, loc), "GET", {}
            continue
        r.final_url = u
        return r
    raise RuntimeError("too many redirects")


def kc_error(text):
    for pat in (r'kc-feedback-text[^"]*"[^>]*>\s*([^<\s][^<]*)<', r'id="input-error[^"]*"[^>]*>\s*([^<\s][^<]*)<',
                r'id="kc-error-message"[^>]*>\s*<p[^>]*>\s*([^<\s][^<]*)<', r'class="instruction"[^>]*>\s*([^<\s][^<]*)<'):
        m = re.search(pat, text)
        if m:
            return html.unescape(m.group(1)).strip()
    return ""


try:
    r0 = s.get(url, allow_redirects=False, timeout=15)
    out["first"] = {"status": r0.status_code, "location": r0.headers.get("Location", "")}
    s.cookies.clear()
    r = go("GET", url)
    b = re.search(r'<form[^>]*action="([^"]*oauth2/start[^"]*)"', r.text)
    if b and "kc-form-login" not in r.text:
        rd = re.search(r'name="rd"\s+value="([^"]*)"', r.text)
        out["proxy_button"] = True
        start = urljoin(r.final_url, html.unescape(b.group(1)))
        r = go("GET", start, params={"rd": html.unescape(rd.group(1))} if rd else None)
    m = re.search(r'<form[^>]*id="kc-form-login"[^>]*action="([^"]+)"', r.text)
    if m:
        out["login_page"] = True
        r = go("POST", html.unescape(m.group(1)), data={"username": user, "password": pw, "credentialId": ""})
    out.update(status=r.status_code, url=r.final_url, body=r.text[:30000])
    if "/realms/" in r.final_url:
        out["kc_error"] = kc_error(r.text)
except Exception as e:
    out.update(status=0, url="", body="", error=f"{type(e).__name__}: {e}")
print(json.dumps(out))
