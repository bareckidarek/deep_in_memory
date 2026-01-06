import os
import sys

print(f"PID procesu: {os.getpid()}", flush=True)

while True:
    ch = sys.stdin.read(1)
    if ch == "":  # EOF
        break
    try:
        b = bool()
    except MemoryError:
        print("malloc failed", file=sys.stderr)
        sys.exit(1)
    b = None  # GC zajmie się resztą