# Hostile: never finish, and ignore polite requests to stop.
import os, signal, time
signal.signal(signal.SIGTERM, signal.SIG_IGN)
signal.signal(signal.SIGINT, signal.SIG_IGN)
n = 0
while True:
    n += 1
    with open("/out/beat", "w") as f:
        f.write(f"{n} {time.time()}\n")
    time.sleep(0.5)
