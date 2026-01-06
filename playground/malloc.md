# Plan: Śledzenie malloc system-wide

TL;DR: Jednofajlowy plan do narzędzia Python+BCC, które system-wide (wszystkie procesy) podczepi uprobes/uretprobes na `malloc` w libc, zapisze żądaną wielkość i zwrócony wskaźnik oraz wypisze timestamp, PID, TID, comm, size i ret_ptr. Służy jako instrukcja dla agenta AI do wygenerowania skryptu `malloc_trace.py`.

## Cele
- Zakres: system-wide (wszystkie procesy) domyślnie, z opcją filtrowania po PID.
- Log: timestamp, PID, TID, comm, requested_size, returned_pointer.
- Domyślne zachowanie: brak samplingu i stałe parametry bufora; jedyna opcja CLI to `--pid`.

## Wymagania środowiskowe (skrót)
- Architektura: x86_64 (domyślnie).
- Kernel: w praktyce ≥4.4 działa, pełne funkcje lepsze na 5.x; BPF/uprobes musi być w kernelu.
- Pakiety: `bcc` / `python3-bcc`, `clang`, `llvm`, `linux-headers-$(uname -r)`, `libelf-dev`.
- Uprawnienia: zwykle `sudo` (root) lub zdolności CAP_BPF/CAP_PERFMON.

## Struktura BPF (wysoki poziom)
- Mapy:
  - `entries` — `BPF_LRU_HASH` (jeśli dostępne) lub `BPF_HASH`, key = `u64 pid_tgid`, value = `u64 size` (opcjonalnie ts).
  - `events` — `BPF_PERF_OUTPUT` do przekazywania zdarzeń do userspace.
- Event struct (C): zawiera `u64 pid_tgid; u32 pid; u32 tid; u64 size; u64 ret; u64 ts_ns; char comm[TASK_COMM_LEN];`.
- Probes:
  - `probe_malloc_entry` (uprobe): odczyt `size = (u64)PT_REGS_PARM1(ctx)` i `entries.update(&key, &size)`.
  - `probe_malloc_ret` (uretprobe): odczyt `ret = (u64)PT_REGS_RC(ctx)`, lookup `size`, emit event, `entries.delete(&key)`.

## Mapy i klucze — szczegóły praktyczne
- Klucz mapy: `u64 key = bpf_get_current_pid_tgid()` (łączy pid i tid).
- Wartość: `u64 size` (8 bajtów wystarczy), można dodać `u64 ts_ns`.
- Rozmiar mapy: domyślnie 65536 wpisów; można obniżyć do 16384/8192.
- Preferuj `BPF_LRU_HASH` żeby uniknąć ręcznej ewikcji, jeśli kernel to wspiera.

## Przechwytywanie rejestrów (x86_64)
- Argument: `PT_REGS_PARM1(ctx)` (RDI) → rzutuj na `u64`.
- Return: `PT_REGS_RC(ctx)` (RAX) → rzutuj na `u64`.
- Uwaga: dla 32-bitowych procesów ABI różni się — narzędzie domyślnie x86_64; dla 32-bit wymagana dodatkowa obsługa.

## Dostarczanie zdarzeń do użytkownika
- Użyj `BPF_PERF_OUTPUT(events)` i w Pythonie `b["events"].open_perf_buffer(callback, page_cnt=PAGE_CNT)`.
- Callback: parsuj zdarzenie przez ctypes.Structure i wypisz pola.
- Page count i limity: page_cnt 8–16 domyślnie; udostępnij opcję `--perf-pages`.

## Obsługa IFUNC / różnych kopii libc
- IFUNC: symbol może być IFUNC i rozwiązywany per-process; jeśli to problem, zaimplementować tryb per-PID:
  - enumerate PIDs, odczytać `/proc/<pid>/maps` aby znaleźć libc path, i attach per-pid (pid argument w BPF.attach_uprobe).
- Domyślnie attach system-wide przez `name` (np. `/lib/.../libc.so.6` lub krótkie `'c'`) z `pid=-1`.

## Wydajność i bezpieczeństwo
- Wysoki event rate → rozważ sampling (`--sample N`) lub LRU mapy.
- Obsłuż brak wpisu w return probe (np. inlined, or exit without entry); nie crashuj.
- Monitoruj i raportuj utracone zdarzenia (perf buffer overflow).
- Prywatność: adresy wskaźników mogą być wrażliwe — dokumentuj i kontroluj dostęp.

## Runtime (Python) — wymagane elementy (bez kodu)
- Importy: `from bcc import BPF`, `ctypes`, `argparse`, `signal`, `time`, `os`, `sys`.
- Ładowanie BPF: `b = BPF(text=PROG)`.
- Attachowanie: `b.attach_uprobe(name=libc_path, sym="malloc", fn_name="probe_malloc_entry", pid=-1)` i analogicznie `attach_uretprobe`.
- Perf buffer: `b["events"].open_perf_buffer(handle_event, page_cnt=...)`, pętla `while True: b.perf_buffer_poll()` z obsługą SIGINT.
- Cleanup: detach lub `b.cleanup()` w handlerze exit.

## CLI i opcje
- `--pid` — śledź tylko dany PID (opcjonalne).

## Testowanie
- Kompilacja prostego programu `test.c` (lokalny program alokujący) i uruchomienie go.
- Uruchomić narzędzie z `sudo python3 malloc_trace.py` i obserwować logi.
- Sprawdzić, że dla każdego wywołania pojawia się linia z timestamp, PID, TID, comm, size, ret_ptr.
- Test IFUNC: uruchomić procesy z LD_PRELOAD lub różnymi libc, użyć `--per-pid`.

## Instrukcje uruchomienia / instalacji (skrót)
- Ubuntu: `sudo apt update && sudo apt install -y bcc python3-bcc linux-headers-$(uname -r) clang llvm libelf-dev`
- Uruchom: `sudo python3 malloc_trace.py` (opcjonalnie `--libc /lib/x86_64-linux-gnu/libc.so.6` lub `--per-pid`).

## Caveats
- Kernel/konfiguracja może blokować BPF/uprobes; wymagane konfiguracje i uprawnienia.
- x86_64-only domyślnie; 32-bit wymaga dodatkowej obsługi.
- System-wide tracing może wytworzyć dużo logów, prowadząc do dropów/podwyższonego zużycia CPU.
- IFUNC i różne biblioteki wymagają per-PID attach lub attach by resolved address.

## Metadane dla agenta AI
- Nazwa docelowa pliku implementacji: `malloc_trace.py`.
- Nazwy funkcji BPF (C): `probe_malloc_entry`, `probe_malloc_ret`.
- Nazwa pliku planu: `malloc.md` (ten plik).
- Format outputu docelowego: CSV lub jedno-liniowe logi: `TS PID TID COMM SIZE RET_PTR`.

---

Plik zapisany. Jeśli chcesz, teraz mogę wygenerować pełny `malloc_trace.py` oparty na tym planie.