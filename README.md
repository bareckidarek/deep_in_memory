# deep_in_memory

See what is really happening inside process memory while your application is running under load.

**deep\_in\_memory** is a Linux terminal user interface (TUI) utility designed to help understand how the Linux kernel manages process memory. It provides a real-time visualization of key memory lifecycle events, including virtual address space reservations, page faults, physical page allocation, and swap activity. The tool uses eBPF tracepoints and kprobes to make kernel memory management mechanisms visible and easier to understand.

## Preview

![TUI preview](docs/preview.png)

## Key Capabilities

*   **Virtual Address Space Monitoring:** Tracks mmap, munmap, and brk syscalls to observe the creation, growth, and removal of memory regions.

*   **Demand Paging Visibility:** Identifies "lazy" physical allocation (***INIT***) by hooking into kernel functions, showing exactly when reserved virtual space becomes backed by RAM.

*   **Swap Lifecycle Tracing:** Monitors the movement of pages to and from swap via page\_out (***SWAP***) and page\_in (***UNSWAP***) events.

*   **Address-to-Physical Resolution:** Allows users to look up specific virtual addresses to retrieve **Page Frame Numbers (PFN)** and cross-reference them with physical hardware ranges in **/proc/iomem**.

*   **Chronological Event Logging:** Provides an aggregated, time-ordered log of memory operations, enabling the reconstruction of a process's memory behavior over time.

*   **Thread-Specific Analysis:** Preserves thread identifiers (TIDs) for memory events, allowing the inspection of memory activity across different execution contexts within a single process.

## Quick start with Docker

```bash
docker run -it -v /sys/kernel/tracing:/sys/kernel/tracing --userns=host --pid=host --privileged bareckidarek/deep_in_memory
```

## Build and run from source

Requirements:

- Linux on amd64
- Kernel 5.8 or newer with BTF support at /sys/kernel/btf/vmlinux
- Root privileges or CAP_BPF plus CAP_PERFMON

Build and run:

```bash
# Debian/Ubuntu
apt-get install -y bpftool clang llvm libbpf-dev

make generate
make build

sudo ./deep_in_memory
```

## Keyboard shortcuts

| Key | Action |
|---|---|
| Up / Down | Navigate process list |
| Enter | Select process and start tracing |
| n / p | Next or previous virtual memory page |
| Tab | Focus physical memory input |
| Enter in input | Run pagemap lookup for current address |
| j / k | Scroll /proc/iomem matches |
| w | Toggle log line wrapping |
| f | Toggle range filter: heap+empty or all |
| q / Ctrl+C | Quit |

## Traced events

| Kernel hook | Event | Meaning |
|---|---|---|
| sched_process_exec | ProcessExec | Process started |
| sched_process_exit | ProcessExit | Process exited |
| sys_enter and sys_exit for clone and clone3 | ProcessClone | Thread or child process creation |
| sys_enter and sys_exit for mmap | ProcessMmap | New virtual memory range |
| sys_enter and sys_exit for munmap | ProcessMunmap | Range unmapped |
| sys_enter and sys_exit for brk | ProcessBrk | Heap boundary change |
| do_anonymous_page | INIT | First page allocation fault |
| swap_writepage | SWAP | Page evicted to swap |
| do_swap_page | UNSWAP | Page read back from swap |


Contributor and agent details: [AGENTS.md](AGENTS.md)
