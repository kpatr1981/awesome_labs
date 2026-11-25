"""ragkit: the small helpers the runbook bot's scripts share.

    from ragkit import connect, embed, embed_query, runbooks

    db = connect()                  # psycopg connection to the rag database, pgvector ready
    vecs = embed(["text", ...])     # one numpy vector (384 floats, length 1) per text
    q = embed_query("question")     # the same for a search question (adds bge's query prefix)
    for rb in runbooks():           # every /lab/runbooks/*.md, sorted by file name
        rb["id"], rb["title"], rb["meta"], rb["body"], rb["path"]

Settings come from the environment, with the lab's defaults:
    RAG_PG   postgres connection string  (host=127.0.0.1 dbname=rag user=postgres password=lab)
    RAG_EMB  embeddings API base URL     (http://127.0.0.1:8081/v1)
"""
import glob
import os

import numpy as np
import psycopg
from openai import OpenAI
from pgvector.psycopg import register_vector

PG = os.environ.get("RAG_PG", "host=127.0.0.1 dbname=rag user=postgres password=lab")
EMB_URL = os.environ.get("RAG_EMB", "http://127.0.0.1:8081/v1")
# bge models expect this instruction in front of short search queries (not in front of documents).
QUERY_PREFIX = "Represent this sentence for searching relevant passages: "

_emb = OpenAI(base_url=EMB_URL, api_key="none")


def connect():
    db = psycopg.connect(PG, autocommit=True)
    db.execute("CREATE EXTENSION IF NOT EXISTS vector")
    register_vector(db)
    return db


def embed(texts):
    """Embed a list of texts in one request; returns a list of numpy vectors."""
    if not texts:
        return []
    res = _emb.embeddings.create(model="bge-small-en-v1.5", input=list(texts))
    return [np.array(d.embedding, dtype=np.float32) for d in res.data]


def embed_query(question):
    return embed([QUERY_PREFIX + question])[0]


def read_runbook(path):
    text = open(path, encoding="utf-8").read()
    meta = {}
    if text.startswith("---\n") and "\n---\n" in text[4:]:
        head, text = text[4:].split("\n---\n", 1)
        for line in head.splitlines():
            k, _, v = line.partition(":")
            if k.strip():
                meta[k.strip()] = v.strip()
    rid = meta.get("id") or os.path.basename(path)[:-3]
    title = meta.get("title", rid)
    return dict(id=rid, title=title, meta=meta, body=text.strip(), path=path)


def runbooks(folder="/lab/runbooks"):
    return [read_runbook(p) for p in sorted(glob.glob(os.path.join(folder, "*.md")))]
