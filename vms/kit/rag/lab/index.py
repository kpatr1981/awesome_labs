"""Index /lab/runbooks into table chunks: one row per ## section of each runbook."""
import re
from ragkit import connect, embed, runbooks

def sections(rb):
    out = []
    for part in re.split(r"\n(?=## )", rb["body"]):
        part = part.strip()
        if part.startswith("## "):
            out.append((part.splitlines()[0][3:].strip(), f"{rb['title']}\n{part}"))
    return out

db = connect()
db.execute("DROP TABLE IF EXISTS chunks")
db.execute("""CREATE TABLE chunks (id text PRIMARY KEY, doc text NOT NULL, heading text,
              body text NOT NULL, embedding vector(384) NOT NULL)""")
for rb in runbooks():
    secs = sections(rb)
    for i, ((heading, body), vec) in enumerate(zip(secs, embed([b for _, b in secs]))):
        db.execute("INSERT INTO chunks VALUES (%s, %s, %s, %s, %s)", (f"{rb['id']}#{i}", rb["id"], heading, body, vec))
print("indexed", db.execute("SELECT count(DISTINCT doc), count(*) FROM chunks").fetchone(), "(runbooks, chunks)")
