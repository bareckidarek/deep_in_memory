import argparse
import struct
import sys

def parse_args():
    parser = argparse.ArgumentParser(description="Odczytaj PFN z /proc/PID/pagemap dla podanego adresu wirtualnego")
    parser.add_argument("--pid", type=int, required=True, help="PID procesu")
    parser.add_argument("--vaddr", type=lambda x: int(x, 0), required=True, help="Adres wirtualny (np. 0x7fff5fbff000)")
    return parser.parse_args()

def main():
    args = parse_args()
    pid = args.pid
    vaddr = args.vaddr

    pagesize = 4096
    entry_size = 8
    page_index = vaddr // pagesize

    try:
        with open(f"/proc/{pid}/pagemap", "rb") as f:
            f.seek(page_index * entry_size)
            entry = f.read(entry_size)
            if len(entry) != entry_size:
                print("Nie udało się odczytać wpisu z pagemap.")
                sys.exit(1)
            value = struct.unpack("Q", entry)[0]
            present = (value >> 63) & 1
            pfn = value & ((1 << 55) - 1)
            print(f"present={present}, pfn={hex(pfn)}")
    except Exception as e:
        print(f"Błąd: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()