"""BM25 keyword search over table chunks.

    bm25.rank_chunks(question, k=20, db=None, where="true") -> [(chunk id, doc, body, score)], best first
    bm25.search(question, k=20, db=None, where="true")      -> runbook ids, best first
    ragpy bm25.py "question"                               -> the 3 best runbook ids (JSON)
`where` is an SQL condition on chunks, e.g. "doc <> 'x'".
"""
import collections, json, math, re, sys
from ragkit import connect

TOKEN = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*")
STOP = set("a an and are as at be by do does for from how i if in is it its my of on or so that "
           "the this to was we what when where which who why will with you your".split())


def tokens(text):
    return [t for t in TOKEN.findall(text.lower()) if t not in STOP]


def rank_chunks(question, k=20, db=None, where="true", k1=1.2, b=0.75):
    rows = (db or connect()).execute(f"SELECT id, doc, body FROM chunks WHERE {where}").fetchall()
    if not rows:
        return []
    docs = [tokens(r[2]) for r in rows]
    n, avg = len(docs), sum(map(len, docs)) / len(docs)
    df = collections.Counter(t for d in docs for t in set(d))
    q = set(tokens(question))
    scored = []
    for (cid, doc, body), d in zip(rows, docs):
        tf = collections.Counter(d)
        s = sum(math.log(1 + (n - df[t] + 0.5) / (df[t] + 0.5)) * tf[t] * (k1 + 1)
                / (tf[t] + k1 * (1 - b + b * len(d) / avg)) for t in q if t in tf)
        if s > 0:
            scored.append((cid, doc, body, s))
    return sorted(scored, key=lambda r: r[3], reverse=True)[:k]


def search(question, k=20, db=None, where="true"):
    return list(dict.fromkeys(r[1] for r in rank_chunks(question, k * 3, db, where)))[:k]


if __name__ == "__main__":
    print(json.dumps(search(sys.argv[1])[:3]))
