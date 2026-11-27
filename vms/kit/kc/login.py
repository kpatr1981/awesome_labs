"""A scripted browser login (Python standard library only, so it runs anywhere).

  python3 login.py URL USER PASSWORD     -> JSON on stdout:
    {"status": final HTTP status, "url": final URL, "body": the page (cut at 30000 chars),
     "login_page": whether a login form was filled in, "kc_error": the identity provider's error
     text, "first": {"status", "location"} of the very first request, "hops": the URLs visited}

It opens URL like a browser: it follows redirects by hand, keeps cookies, and presses
oauth2-proxy's "Sign in with ..." button if that page comes up. On a page with a login form (a
form with a password field: Keycloak's, Dex's...), it fills in USER and PASSWORD, keeps the
form's hidden fields, submits it, and follows the redirects back. Browsers send Secure cookies
to http://127.0.0.1 and localhost (they count as secure origins); Python doesn't, so the
Secure flag is dropped from every cookie received.
"""
import html, http.cookiejar, json, re, sys, urllib.error, urllib.parse, urllib.request
from html.parser import HTMLParser

url, user, pw = sys.argv[1], sys.argv[2], sys.argv[3]
jar = http.cookiejar.CookieJar()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **k):
        return None


opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect)
out = {"login_page": False, "kc_error": "", "hops": []}


class Page:
    def __init__(self, status, url, headers, text):
        self.status, self.url, self.headers, self.text = status, url, headers, text


def fetch(method, u, data=None):
    body = urllib.parse.urlencode(data).encode() if data is not None else None
    req = urllib.request.Request(u, data=body, method=method, headers={"User-Agent": "kl-login/1"})
    if body is not None:
        req.add_header("Content-Type", "application/x-www-form-urlencoded")
    try:
        r = opener.open(req, timeout=15)
    except urllib.error.HTTPError as e:
        r = e
    text = r.read().decode("utf-8", "replace")
    for c in jar:
        c.secure = False
    return Page(r.status if hasattr(r, "status") else r.code, u, r.headers, text)


def go(method, u, data=None):
    for _ in range(25):
        p = fetch(method, u, data)
        out["hops"].append(f"{p.status} {u[:120]}")
        loc = p.headers.get("Location")
        if p.status in (301, 302, 303, 307, 308) and loc:
            u, method, data = urllib.parse.urljoin(u, loc), "GET", None
            continue
        return p
    raise RuntimeError("too many redirects")


class Forms(HTMLParser):
    """Collect forms: action, method and inputs (name, type, value)."""
    def __init__(self):
        super().__init__()
        self.forms, self.cur = [], None

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if tag == "form":
            self.cur = {"action": a.get("action") or "", "method": (a.get("method") or "get").lower(), "inputs": []}
            self.forms.append(self.cur)
        elif tag in ("input", "button") and self.cur is not None and a.get("name"):
            self.cur["inputs"].append((a["name"], (a.get("type") or "text").lower(), a.get("value") or ""))

    def handle_endtag(self, tag):
        if tag == "form":
            self.cur = None


def forms(text):
    f = Forms()
    f.feed(text)
    return f.forms


def error_text(text):
    for pat in (r'kc-feedback-text[^"]*"[^>]*>\s*([^<\s][^<]*)<', r'id="input-error[^"]*"[^>]*>\s*([^<\s][^<]*)<',
                r'id="kc-error-message"[^>]*>\s*<p[^>]*>\s*([^<\s][^<]*)<', r'class="instruction"[^>]*>\s*([^<\s][^<]*)<',
                r'id="login-error"[^>]*>\s*([^<\s][^<]*)<', r'class="dex-error-box"[^>]*>\s*([^<\s][^<]*)<'):
        m = re.search(pat, text)
        if m:
            return html.unescape(m.group(1)).strip()
    return ""


try:
    first = fetch("GET", url)
    out["first"] = {"status": first.status, "location": first.headers.get("Location", "")}
    jar.clear()
    p = go("GET", url)
    for f in forms(p.text):
        if "oauth2/start" in f["action"] and not any(t == "password" for _, t, _ in f["inputs"]):
            out["proxy_button"] = True
            q = {n: v for n, t, v in f["inputs"] if t == "hidden"}
            target = urllib.parse.urljoin(p.url, html.unescape(f["action"]))
            p = go("GET", target + ("&" if "?" in target else "?") + urllib.parse.urlencode(q) if q else target)
            break
    for f in forms(p.text):
        if any(t == "password" for _, t, _ in f["inputs"]):
            out["login_page"] = True
            data = {}
            for n, t, v in f["inputs"]:
                if t == "password":
                    data[n] = pw
                elif t in ("text", "email") or n in ("username", "login"):
                    data[n] = user
                elif t in ("hidden",):
                    data[n] = v
            target = urllib.parse.urljoin(p.url, html.unescape(f["action"])) if f["action"] else p.url
            p = go("POST", target, data)
            break
    out.update(status=p.status, url=p.url, body=p.text[:30000])
    if not out["kc_error"] and (any(t == "password" for f in forms(p.text) for _, t, _ in f["inputs"]) or p.status >= 400):
        out["kc_error"] = error_text(p.text)
except Exception as e:
    out.update(status=0, url="", body="", error=f"{type(e).__name__}: {e}")
print(json.dumps(out))
