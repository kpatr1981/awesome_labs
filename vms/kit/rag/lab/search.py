"""Hybrid search: vector + BM25, fused with weighted reciprocal rank fusion.

    search.search(question, db=None, where="true") -> runbook ids, best first
    ragpy search.py "question"                    -> the 3 best (JSON)
"""
import json, sys
import bm25
from ragkit import connect, embed_query

def vector(question, k=20, db=None, where="true"):
    rows = (db or connect()).execute(f"SELECT doc FROM chunks WHERE {where} ORDER BY embedding <=> %s LIMIT %s",
                                     (embed_query(question), k * 3)).fetchall()
    return list(dict.fromkeys(r[0] for r in rows))[:k]

def rrf(lists, weights, k=60):
    score = {}
    for ranked, w in zip(lists, weights):
        for rank, doc in enumerate(ranked, 1):
            score[doc] = score.get(doc, 0) + w / (k + rank)
    return sorted(score, key=score.get, reverse=True)

def search(question, db=None, where="true"):
    db = db or connect()
    return rrf([vector(question, db=db, where=where), bm25.search(question, db=db, where=where)], [1.0, 2.0])

if __name__ == "__main__":
    print(json.dumps(search(sys.argv[1])[:3]))
