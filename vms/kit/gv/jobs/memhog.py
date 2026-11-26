# Hostile: take all the memory there is, 16 MiB at a time.
import time
blocks = []
for i in range(1, 129):
    b = bytearray(16 << 20)
    for j in range(0, len(b), 4096):
        b[j] = 1
    blocks.append(b)
    print(f"holding {i * 16} MiB", flush=True)
time.sleep(2)
print("got 2 GiB")
