"""Run the learner's search on hidden questions (AI engineering, Retrieval labs).

  python eval.py QUESTIONS.jsonl OUT.json -- CMD ARG...

Runs CMD ARG... [the question's "args"...] "<question>" for every question (in /lab), reads the JSON list of runbook
ids it prints (a chunk id "doc#3" counts as "doc"; objects with an "id" key work too) and
writes OUT.json: {"results": [{q, want, kind, got, rank, rc, err, raw}], "n", "hit1", "hit3", "mrr"};
raw is the last line printed (the whole JSON answer).
rank is the 1-based position of the first wanted id in got, or 0.
"""
import json, subprocess, sys


def ids(out):
    try:
        v = json.loads(out.strip().splitlines()[-1])
    except Exception:
        return None
    if isinstance(v, dict):
        v = v.get("results") or v.get("ids") or v.get("sources") or []
    got = []
    for x in v if isinstance(v, list) else []:
        if isinstance(x, dict):
            x = x.get("id") or x.get("doc") or ""
        x = str(x).split("#")[0]
        if x.endswith(".md"):
            x = x[:-3]
        got.append(x)
    return got


def main():
    qf, outf = sys.argv[1], sys.argv[2]
    cmd = sys.argv[sys.argv.index("--") + 1:]
    qs = [json.loads(l) for l in open(qf) if l.strip()]
    res = []
    for q in qs:
        try:
            p = subprocess.run(cmd + q.get("args", []) + [q["q"]], cwd="/lab", capture_output=True, text=True, timeout=60)
            rc, out, err = p.returncode, p.stdout, p.stderr[-400:]
        except subprocess.TimeoutExpired:
            rc, out, err = 124, "", "timeout"
        got = ids(out) if rc == 0 else None
        rank = 0
        for i, g in enumerate(got or []):
            if g in q["want"]:
                rank = i + 1
                break
        last = out.strip().splitlines()[-1][:4000] if out.strip() else ""
        res.append(dict(q, got=got, rank=rank, rc=rc, err=err if rc else "", raw=last))
    scored = [r for r in res if r["want"]]
    n = len(scored) or 1
    summary = dict(
        results=res, n=len(scored),
        hit1=sum(r["rank"] == 1 for r in scored) / n,
        hit3=sum(1 <= r["rank"] <= 3 for r in scored) / n,
        mrr=sum(1 / r["rank"] for r in scored if r["rank"]) / n,
    )
    json.dump(summary, open(outf, "w"), indent=1)


main()
