#!/usr/bin/env python3
"""Trace libc malloc entry/exit and free entry with BCC/eBPF.

Outputs separate events for malloc (entry/ret) and free (entry) with timestamp, PID,
TID, comm, sizes/ptrs. Defaults to system-wide and accepts an optional PID filter.
"""
import argparse
import ctypes
import os
import sys
import time
from datetime import datetime

from bcc import BPF

PERF_PAGES = 8


def build_bpf_text() -> str:
    return """
#include <uapi/linux/ptrace.h>
#include <linux/sched.h>
#include <uapi/linux/bpf.h>
#include <bcc/proto.h>
#include <linux/mm.h>

#define MAP_SIZE           65536
#define SIZE_SZ            8
#define MALLOC_ALIGNMENT   16
#define MALLOC_ALIGN_MASK  15
#define MINSIZE            32

#define request2size(req)                                         \
  (((req) + SIZE_SZ + MALLOC_ALIGN_MASK < MINSIZE)  ?             \
   MINSIZE :                                                      \
   ((req) + SIZE_SZ + MALLOC_ALIGN_MASK) & ~MALLOC_ALIGN_MASK)

struct vm_area_struct;
struct vm_fault_partial {
        struct vm_area_struct *vma;
        unsigned int flags;
        unsigned long pgoff;
        void *address;
};

struct malloc_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 size;
    u64 real_size;
    u64 ret;
    u64 ts_ns;
    char comm[TASK_COMM_LEN];
    u64 ptr;
};

struct free_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    char comm[TASK_COMM_LEN];
    u64 ptr;
};

struct brk_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    u64 addr;
    u64 ret;
    char comm[TASK_COMM_LEN];
};

struct mmap_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    u64 addr;      // return value from exit
    u64 len;
    u64 prot;
    u64 flags;
    u64 fd;
    u64 pgoff;
    u64 ret;
    char comm[TASK_COMM_LEN];
};

struct munmap_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    u64 addr;
    u64 len;
    u64 ret;
    char comm[TASK_COMM_LEN];
};

struct page_alloc_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    unsigned long pfn;
    u32 order;
    u32 gfp_flags;
    int migratetype;
};

struct page_free_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    unsigned long pfn;
    u32 order;
};

struct page_fault_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    u64 address;
    u64 ip;
    u64 error_code;
    char comm[TASK_COMM_LEN];
};

struct swap_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    u64 address;
    u64 pgoff;
    u64 ret;
    char comm[TASK_COMM_LEN];
};

struct swap_writepage_event_t {
    u64 pid_tgid;
    u32 pid;
    u32 tid;
    u64 ts_ns;
    u64 page_ptr;
    u64 vaddr;
    u64 ret;
    char comm[TASK_COMM_LEN];
};

struct tracepoint__mm__page_alloc {
    unsigned long pfn;
    unsigned int order;
    unsigned int gfp_flags;
    int migratetype;
};

struct tracepoint__mm__page_free {
    unsigned long pfn;
    unsigned int order;
};

BPF_HASH(entries, u64, u64, MAP_SIZE);
BPF_HASH(brk_entries, u64, struct brk_event_t, MAP_SIZE);
BPF_HASH(mmap_entries, u64, struct mmap_event_t, MAP_SIZE);
BPF_HASH(munmap_entries, u64, struct munmap_event_t, MAP_SIZE);
BPF_PERF_OUTPUT(malloc_events);
BPF_PERF_OUTPUT(free_events);
BPF_PERF_OUTPUT(brk_events);
BPF_PERF_OUTPUT(mmap_events);
BPF_PERF_OUTPUT(munmap_events);
BPF_PERF_OUTPUT(page_alloc_events);
BPF_PERF_OUTPUT(page_free_events);
BPF_PERF_OUTPUT(page_fault_events);
BPF_PERF_OUTPUT(swap_events);
BPF_PERF_OUTPUT(swap_writepage_events);
BPF_ARRAY(filter_pid, u32, 1);
BPF_HASH(swap_entries, u64, struct swap_event_t, MAP_SIZE);
BPF_HASH(swap_writepage_entries, u64, struct swap_writepage_event_t, MAP_SIZE);

int probe_malloc_entry(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }
    u64 size = (u64)PT_REGS_PARM1(ctx);
    entries.update(&key, &size);
    return 0;
}

int probe_malloc_ret(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }
    u64 *sizep = entries.lookup(&key);
    if (!sizep) {
        return 0;
    }

    struct malloc_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.size = *sizep;
    evt.real_size = request2size(*sizep);
    evt.ret = (u64)PT_REGS_RC(ctx);
    evt.ptr = evt.ret;
    evt.ts_ns = bpf_ktime_get_ns();
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));

    malloc_events.perf_submit(ctx, &evt, sizeof(evt));
    entries.delete(&key);
    return 0;
}

int probe_free_entry(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct free_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ptr = (u64)PT_REGS_PARM1(ctx);
    evt.ts_ns = bpf_ktime_get_ns();
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));

    free_events.perf_submit(ctx, &evt, sizeof(evt));
    return 0;
}

TRACEPOINT_PROBE(syscalls, sys_enter_brk) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct brk_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();
    evt.addr = args->brk;
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));

    brk_entries.update(&key, &evt);
    return 0;
}

TRACEPOINT_PROBE(syscalls, sys_exit_brk) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct brk_event_t *entry = brk_entries.lookup(&key);
    if (!entry) {
        return 0;
    }

    entry->ret = args->ret;
    brk_events.perf_submit(args, entry, sizeof(*entry));
    brk_entries.delete(&key);
    return 0;
}

TRACEPOINT_PROBE(syscalls, sys_enter_mmap) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct mmap_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();
    evt.len = args->len;
    evt.prot = args->prot;
    evt.flags = args->flags;
    evt.fd = args->fd;
    evt.pgoff = args->off;
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));

    mmap_entries.update(&key, &evt);
    return 0;
}

TRACEPOINT_PROBE(syscalls, sys_exit_mmap) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct mmap_event_t *entry = mmap_entries.lookup(&key);
    if (!entry) {
        return 0;
    }

    entry->addr = args->ret;
    entry->ret = args->ret;
    mmap_events.perf_submit(args, entry, sizeof(*entry));
    mmap_entries.delete(&key);
    return 0;
}

TRACEPOINT_PROBE(syscalls, sys_enter_munmap) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct munmap_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();
    evt.addr = args->addr;
    evt.len = args->len;
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));

    munmap_entries.update(&key, &evt);
    return 0;
}

TRACEPOINT_PROBE(syscalls, sys_exit_munmap) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct munmap_event_t *entry = munmap_entries.lookup(&key);
    if (!entry) {
        return 0;
    }

    entry->ret = args->ret;
    munmap_events.perf_submit(args, entry, sizeof(*entry));
    munmap_entries.delete(&key);
    return 0;
}

TRACEPOINT_PROBE(kmem, mm_page_alloc) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct page_alloc_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();
    evt.pfn = args->pfn;
    evt.order = args->order;
    evt.gfp_flags = args->gfp_flags;
    evt.migratetype = args->migratetype;

    page_alloc_events.perf_submit(args, &evt, sizeof(evt));
    return 0;
}

TRACEPOINT_PROBE(kmem, mm_page_free) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct page_free_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();
    evt.pfn = args->pfn;
    evt.order = args->order;

    page_free_events.perf_submit(args, &evt, sizeof(evt));
    return 0;
}

TRACEPOINT_PROBE(exceptions, page_fault_user) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct page_fault_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();
    evt.address = args->address;
    evt.ip = args->ip;
    evt.error_code = args->error_code;
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));

    page_fault_events.perf_submit(args, &evt, sizeof(evt));
    return 0;
}

int kprobe__do_swap_page(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct swap_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();

    struct vm_fault_partial vmf = {};
    struct vm_fault_partial *vmf_ptr = (struct vm_fault_partial *)PT_REGS_PARM1(ctx);
    if (vmf_ptr) {
        bpf_probe_read_kernel(&vmf, sizeof(vmf), vmf_ptr);
        evt.address = (u64)vmf.address;
        evt.pgoff = (u64)vmf.pgoff;
    }

    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));
    swap_entries.update(&key, &evt);
    return 0;
}

int kretprobe__do_swap_page(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct swap_event_t *entry = swap_entries.lookup(&key);
    if (!entry) {
        return 0;
    }

    entry->ret = PT_REGS_RC(ctx);
    swap_events.perf_submit(ctx, entry, sizeof(*entry));
    swap_entries.delete(&key);
    return 0;
}

int kprobe__swap_writepage(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct swap_writepage_event_t evt = {};
    evt.pid_tgid = key;
    evt.pid = key >> 32;
    evt.tid = (u32)key;
    evt.ts_ns = bpf_ktime_get_ns();
    struct page *page = (struct page *)PT_REGS_PARM1(ctx);
    evt.page_ptr = (u64)page;
    if (page) {
        unsigned long idx_val = 0;
        bpf_probe_read_kernel(&idx_val, sizeof(idx_val), &page->index);
        evt.vaddr = (u64)idx_val;
    }
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));
    swap_writepage_entries.update(&key, &evt);
    return 0;
}

int kretprobe__swap_writepage(struct pt_regs *ctx) {
    u64 key = bpf_get_current_pid_tgid();
    u32 pid = key >> 32;
    u32 idx = 0;
    u32 *target = filter_pid.lookup(&idx);
    if (target && *target && pid != *target) {
        return 0;
    }

    struct swap_writepage_event_t *entry = swap_writepage_entries.lookup(&key);
    if (!entry) {
        return 0;
    }

    entry->ret = PT_REGS_RC(ctx);
    swap_writepage_events.perf_submit(ctx, entry, sizeof(*entry));
    swap_writepage_entries.delete(&key);
    return 0;
}
"""


class MallocEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("size", ctypes.c_ulonglong),
        ("real_size", ctypes.c_ulonglong),
        ("ret", ctypes.c_ulonglong),
        ("ts_ns", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
        ("ptr", ctypes.c_ulonglong),
    ]


class FreeEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
        ("ptr", ctypes.c_ulonglong),
    ]


class BrkEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("addr", ctypes.c_ulonglong),
        ("ret", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
    ]


class MmapEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("addr", ctypes.c_ulonglong),
        ("len", ctypes.c_ulonglong),
        ("prot", ctypes.c_ulonglong),
        ("flags", ctypes.c_ulonglong),
        ("fd", ctypes.c_ulonglong),
        ("pgoff", ctypes.c_ulonglong),
        ("ret", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
    ]


class MunmapEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("addr", ctypes.c_ulonglong),
        ("len", ctypes.c_ulonglong),
        ("ret", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
    ]


class PageAllocEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("pfn", ctypes.c_ulong),
        ("order", ctypes.c_uint),
        ("gfp_flags", ctypes.c_uint),
        ("migratetype", ctypes.c_int),
    ]


class PageFreeEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("pfn", ctypes.c_ulong),
        ("order", ctypes.c_uint),
    ]


class PageFaultEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("address", ctypes.c_ulonglong),
        ("ip", ctypes.c_ulonglong),
        ("error_code", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
    ]


class SwapEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("address", ctypes.c_ulonglong),
        ("pgoff", ctypes.c_ulonglong),
        ("ret", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
    ]


class SwapWritepageEvent(ctypes.Structure):
    _fields_ = [
        ("pid_tgid", ctypes.c_ulonglong),
        ("pid", ctypes.c_uint),
        ("tid", ctypes.c_uint),
        ("ts_ns", ctypes.c_ulonglong),
        ("page_ptr", ctypes.c_ulonglong),
        ("vaddr", ctypes.c_ulonglong),
        ("ret", ctypes.c_ulonglong),
        ("comm", ctypes.c_char * 16),
    ]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Trace malloc entry/exit system-wide using BCC/eBPF",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    parser.add_argument("--pid", type=int, help="Attach only to this PID", default=-1)
    return parser.parse_args()


def main():
    args = parse_args()

    libc_path = "/lib/x86_64-linux-gnu/libc.so.6"
    pid_attach = args.pid if args.pid > 0 else -1

    bpf_text = build_bpf_text()
    b = BPF(text=bpf_text)

    # Set PID filter for tracepoint-based probes (0 disables filter).
    filter_idx = ctypes.c_int(0)
    filter_val = ctypes.c_uint(pid_attach if pid_attach > 0 else 0)
    b["filter_pid"].__setitem__(filter_idx, filter_val)

    b.attach_uprobe(name=libc_path, sym="malloc", fn_name="probe_malloc_entry", pid=pid_attach)
    b.attach_uretprobe(name=libc_path, sym="malloc", fn_name="probe_malloc_ret", pid=pid_attach)
    b.attach_uprobe(name=libc_path, sym="free", fn_name="probe_free_entry", pid=pid_attach)

    mono_base_ns = time.monotonic_ns()
    wall_base_sec = time.time()

    def format_ts(ts_ns: int) -> str:
        nonlocal mono_base_ns, wall_base_sec
        delta_ns = ts_ns - mono_base_ns
        wall_ts = wall_base_sec + (delta_ns / 1_000_000_000.0)
        return datetime.fromtimestamp(wall_ts).strftime("%Y-%m-%d %H:%M:%S")

    # Track mmap ranges and counters for page_fault and swap_writepage
    mmap_ranges = []  # List of (start, end) tuples
    page_fault_counters = {}  # {range_idx: count}
    swap_writepage_counters = {}  # {range_idx: count}
    do_swap_page_counters = {}  # {range_idx: count}

    def find_range_for_addr(addr):
        """Find which mmap range contains the given address."""
        for idx, (start, end) in enumerate(mmap_ranges):
            if start <= addr < end:
                return idx
        return None

    def print_ranges_summary():
        """Print summary of all ranges with their counters."""
        if not mmap_ranges:
            return ""
        lines = []
        for idx, (start, end) in enumerate(mmap_ranges):
            pf_count = page_fault_counters.get(idx, 0)
            sw_count = swap_writepage_counters.get(idx, 0)
            ds_count = do_swap_page_counters.get(idx, 0)
            lines.append(f"[0x{start:x} - 0x{end:x}, pf:{pf_count}, sw:{sw_count}, ds:{ds_count}]")
        return "\n".join(lines)

    def handle_malloc_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(MallocEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"malloc comm={comm} size={event.size} real_size={event.real_size} ret=0x{event.ret:x}"
        )

    def handle_free_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(FreeEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"free comm={comm} free_ptr=0x{event.ptr:x}")

    def handle_brk_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(BrkEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"brk comm={comm} brk=0x{event.addr:x} ret=0x{event.ret:x}"
        )

    def handle_mmap_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(MmapEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        
        # Track mmap range
        if event.ret != 0 and event.ret != 0xffffffffffffffff:  # Valid mmap
            start = event.ret
            end = event.ret + event.len
            mmap_ranges.append((start, end))
        
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"mmap comm={comm} len=0x{event.len:x} "
            f"prot=0x{event.prot:x} flags=0x{event.flags:x} fd={event.fd} pgoff={event.pgoff} ret=0x{event.ret:x}"
        )

    def handle_munmap_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(MunmapEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"munmap comm={comm} addr=0x{event.addr:x} len={event.len} ret=0x{event.ret:x}"
        )

    def handle_page_alloc_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(PageAllocEvent)).contents
        ts_fmt = format_ts(event.ts_ns)
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"page_alloc pfn=0x{event.pfn:x} order={event.order} gfp=0x{event.gfp_flags:x} migratetype={event.migratetype}"
        )

    def handle_page_free_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(PageFreeEvent)).contents
        ts_fmt = format_ts(event.ts_ns)
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"page_free pfn=0x{event.pfn:x} order={event.order}"
        )

    def handle_page_fault_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(PageFaultEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        
        # Track page fault in range
        range_idx = find_range_for_addr(event.address)
        if range_idx is not None:
            page_fault_counters[range_idx] = page_fault_counters.get(range_idx, 0) + 1
        
        # Print with ranges summary
        ranges_info = print_ranges_summary()
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"page_fault_user comm={comm} addr=0x{event.address:x} ip=0x{event.ip:x} error=0x{event.error_code:x}\n"
            f"{ranges_info}"
        )

    def handle_swap_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(SwapEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        
        # Track do_swap_page in range
        if event.ret == 0:  # Only count successful swaps
            range_idx = find_range_for_addr(event.address)
            if range_idx is not None:
                do_swap_page_counters[range_idx] = do_swap_page_counters.get(range_idx, 0) + 1
        
        # Print with ranges summary
        ranges_info = print_ranges_summary()
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"do_swap_page comm={comm} addr=0x{event.address:x} pgoff=0x{event.pgoff:x} ret=0x{event.ret:x}\n"
            f"{ranges_info}"
        )

    def handle_swap_writepage_event(cpu, data, size):
        event = ctypes.cast(data, ctypes.POINTER(SwapWritepageEvent)).contents
        comm = event.comm.split(b"\0", 1)[0].decode(errors="replace")
        ts_fmt = format_ts(event.ts_ns)
        
        # vaddr needs to be shifted left by 12 bits (add 000 at the end)
        actual_addr = event.vaddr << 12
        
        # Track swap_writepage in range
        if event.ret == 0:  # Only count successful writepages
            range_idx = find_range_for_addr(actual_addr)
            if range_idx is not None:
                swap_writepage_counters[range_idx] = swap_writepage_counters.get(range_idx, 0) + 1
        
        # Print with ranges summary
        ranges_info = print_ranges_summary()
        print(
            f"{ts_fmt} pid={event.pid} tid={event.tid} "
            f"swap_writepage comm={comm} page=0x{event.page_ptr:x} vaddr=0x{actual_addr:x} ret=0x{event.ret:x}\n"
            f"{ranges_info}"
        )

    #b["malloc_events"].open_perf_buffer(handle_malloc_event, page_cnt=PERF_PAGES)
    # b["free_events"].open_perf_buffer(handle_free_event, page_cnt=PERF_PAGES)
    #b["brk_events"].open_perf_buffer(handle_brk_event, page_cnt=PERF_PAGES)
    b["mmap_events"].open_perf_buffer(handle_mmap_event, page_cnt=PERF_PAGES)
    # b["munmap_events"].open_perf_buffer(handle_munmap_event, page_cnt=PERF_PAGES)
    # b["page_alloc_events"].open_perf_buffer(handle_page_alloc_event, page_cnt=PERF_PAGES)
    # b["page_free_events"].open_perf_buffer(handle_page_free_event, page_cnt=PERF_PAGES)
    b["page_fault_events"].open_perf_buffer(handle_page_fault_event, page_cnt=PERF_PAGES)
    b["swap_events"].open_perf_buffer(handle_swap_event, page_cnt=PERF_PAGES)
    b["swap_writepage_events"].open_perf_buffer(handle_swap_writepage_event, page_cnt=PERF_PAGES)

    try:
        while True:
            b.perf_buffer_poll(timeout=1)
    except KeyboardInterrupt:
        print("Detaching...")
    finally:
        b.cleanup()


if __name__ == "__main__":
    if os.geteuid() != 0:
        print("Warning: run as root (sudo) for uprobes to load correctly.", file=sys.stderr)
    main()
