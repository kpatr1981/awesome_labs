# A slow job, so the check can look at its container while it runs.
import time
time.sleep(12)
open("/out/done", "w").write("ok\n")
print("lingered")
