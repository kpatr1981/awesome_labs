# Hostile: start as many processes as the sandbox allows.
import json, os, time
n, err = 0, None
for _ in range(3000):
    try:
        if os.fork() == 0:
            time.sleep(30)
            os._exit(0)
        n += 1
    except OSError as e:
        err = str(e)
        break
json.dump({"forked": n, "error": err}, open("/out/fork.json", "w"))
print("forked", n, err or "")
