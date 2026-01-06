from bcc import BPF

# Program eBPF
bpf_program = """
#include <uapi/linux/ptrace.h>
#include <linux/mm.h>
#include <linux/mm_types.h>
#include <linux/sched.h>

struct data_t {
    u64 pid;
    char comm[TASK_COMM_LEN];
    u64 address;
    u64 pfn;
    u64 error_code;
};

BPF_PERF_OUTPUT(events);

TRACEPOINT_PROBE(exceptions, page_fault_user) {
    struct data_t data = {};
    struct task_struct *task;
    struct mm_struct *mm;
    struct vm_area_struct *vma;
    u64 address = args->address;

    // Pobierz aktualny proces
    task = (struct task_struct *)bpf_get_current_task();
    mm = task->mm;

    // Sprawdź, czy adres wirtualny jest w zakresie
    vma = find_vma(mm, address);
    if (!vma || address < vma->vm_start || address >= vma->vm_end) {
        return 0; // Adres poza zakresem
    }

    // Pobierz PFN (Page Frame Number)
    struct page *page = NULL;
    pte_t *pte;
    spinlock_t *ptl;
    pte = get_locked_pte(mm, address, &ptl);
    if (pte && pte_present(*pte)) {
        page = pte_page(*pte);
        data.pfn = page_to_pfn(page);
    }
    if (ptl) {
        pte_unmap_unlock(pte, ptl);
    }

    // Wypełnij dane do logowania
    data.pid = bpf_get_current_pid_tgid() >> 32;
    bpf_get_current_comm(&data.comm, sizeof(data.comm));
    data.address = address;
    data.error_code = args->error_code;

    // Wyślij dane do przestrzeni użytkownika
    events.perf_submit(args, &data, sizeof(data));
    return 0;
}
"""

# Inicjalizacja BPF
b = BPF(text=bpf_program)

# Definicja formatu wyjścia
def print_event(cpu, data, size):
    event = b["events"].event(data)
    print(f"PID={event.pid}, comm={event.comm.decode()}, "
          f"address=0x{event.address:x}, pfn={event.pfn}, error_code={event.error_code}")

# Subskrybuj zdarzenia
b["events"].open_perf_buffer(print_event)

print("Tracing page faults... Press Ctrl+C to stop.")
while True:
    try:
        b.perf_buffer_poll()
    except KeyboardInterrupt:
        exit()