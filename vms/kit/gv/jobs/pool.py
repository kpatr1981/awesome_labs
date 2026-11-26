# A well-behaved parallel job: 16 worker processes.
import json
from multiprocessing import Pool

def work(n):
    return sum(i * i for i in range(n))

if __name__ == "__main__":
    with Pool(16) as p:
        res = p.map(work, [20000 + i for i in range(64)])
    json.dump({"workers": 16, "total": sum(res)}, open("/out/pool.json", "w"))
    print("pool done")
