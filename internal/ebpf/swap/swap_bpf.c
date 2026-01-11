// +build ignore

#define __TARGET_ARCH_x86
#include "vmlinux_wrapper.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

struct swap_event {
    __u64 ts;
    __u32 type; // 1=do_swap_page, 2=swap_writepage
    __u32 pid;
    __u32 tid;
    __u64 addr;
    __u64 ret;
    char comm[16];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} target_pid SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);
    __type(value, struct swap_event);
} swap_entries SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);
    __type(value, struct swap_event);
} swap_writepage_entries SEC(".maps");

// Minimal vm_fault structure for do_swap_page
struct vm_fault_partial {
    void *vma;
    __u32 flags;
    __u64 pgoff;
    void *address;
};

SEC("kprobe/do_swap_page")
int kprobe_do_swap_page(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    
    __u32 key = 0;
    __u32 *target = bpf_map_lookup_elem(&target_pid, &key);
    if (!target || *target == 0) {
        return 0;
    }
    if (pid != *target) {
        return 0;
    }

    struct swap_event evt = {};
    evt.ts = bpf_ktime_get_ns();
    evt.type = 1; // do_swap_page
    evt.pid = pid;
    evt.tid = (__u32)pid_tgid;
    
    // Read vm_fault argument
    struct vm_fault_partial *vmf = (struct vm_fault_partial *)PT_REGS_PARM1(ctx);
    if (vmf) {
        void *addr_ptr;
        bpf_probe_read_kernel(&addr_ptr, sizeof(addr_ptr), &vmf->address);
        evt.addr = (__u64)addr_ptr;
    }
    
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));
    
    bpf_map_update_elem(&swap_entries, &pid_tgid, &evt, BPF_ANY);
    return 0;
}

SEC("kretprobe/do_swap_page")
int kretprobe_do_swap_page(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    
    __u32 key = 0;
    __u32 *target = bpf_map_lookup_elem(&target_pid, &key);
    if (!target || *target == 0) {
        return 0;
    }
    if (pid != *target) {
        return 0;
    }

    struct swap_event *evt = bpf_map_lookup_elem(&swap_entries, &pid_tgid);
    if (!evt) {
        return 0;
    }

    evt->ret = PT_REGS_RC(ctx);
    
    // Only send if no error (ret == 0)
    if (evt->ret == 0) {
        bpf_ringbuf_output(&events, evt, sizeof(*evt), 0);
    }
    
    bpf_map_delete_elem(&swap_entries, &pid_tgid);
    return 0;
}

SEC("kprobe/swap_writepage")
int kprobe_swap_writepage(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    
    __u32 key = 0;
    __u32 *target = bpf_map_lookup_elem(&target_pid, &key);
    if (!target || *target == 0) {
        return 0;
    }
    if (pid != *target) {
        return 0;
    }

    struct swap_event evt = {};
    evt.ts = bpf_ktime_get_ns();
    evt.type = 2; // swap_writepage
    evt.pid = pid;
    evt.tid = (__u32)pid_tgid;
    
    // Read page->index for virtual address
    struct page *page = (struct page *)PT_REGS_PARM1(ctx);
    if (page) {
        __u64 idx_val;
        bpf_probe_read_kernel(&idx_val, sizeof(idx_val), &page->index);
        evt.addr = idx_val;
    }
    
    bpf_get_current_comm(&evt.comm, sizeof(evt.comm));
    
    bpf_map_update_elem(&swap_writepage_entries, &pid_tgid, &evt, BPF_ANY);
    return 0;
}

SEC("kretprobe/swap_writepage")
int kretprobe_swap_writepage(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    
    __u32 key = 0;
    __u32 *target = bpf_map_lookup_elem(&target_pid, &key);
    if (!target || *target == 0) {
        return 0;
    }
    if (pid != *target) {
        return 0;
    }

    struct swap_event *evt = bpf_map_lookup_elem(&swap_writepage_entries, &pid_tgid);
    if (!evt) {
        return 0;
    }

    evt->ret = PT_REGS_RC(ctx);
    
    // Only send if no error (ret == 0)
    if (evt->ret == 0) {
        bpf_ringbuf_output(&events, evt, sizeof(*evt), 0);
    }
    
    bpf_map_delete_elem(&swap_writepage_entries, &pid_tgid);
    return 0;
}
