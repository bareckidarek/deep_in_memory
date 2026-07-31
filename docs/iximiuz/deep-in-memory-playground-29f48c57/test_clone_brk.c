#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <sys/wait.h>
#define STACK_SIZE (1024 * 1024)
#define ALLOC_SIZE (1024 * 1024)
#define WORKERS 10


struct worker_arg {
    int id;
    int go_fd;   // pipe to synchronize main -> worker
    int done_fd; // pipe to synchronize worker -> main
};

static void wait_for_next_step(const char *step_mode, const char *message) {
    if (step_mode != NULL && strcmp(step_mode, "manual") == 0) {
        printf("%s (press ENTER)\n", message);
        getchar();
        return;
    }

    printf("%s (waiting 5 seconds)\n", message);
    sleep(5);
}

static int clone_flags_for_mode(const char *mode) {
    
    if (strcmp(mode, "shared_vm") == 0) {
        return CLONE_VM | SIGCHLD;
    }

    if (strcmp(mode, "shared_thread") == 0) {
        return CLONE_VM | CLONE_THREAD | CLONE_SIGHAND | CLONE_FS | CLONE_FILES | SIGCHLD;
    }

    return SIGCHLD;
}

static int worker_fn(void *arg) {
    struct worker_arg *w = (struct worker_arg *)arg;
    printf("Worker %d ready (pid=%d, tid=%ld)\n", w->id, getpid(), syscall(SYS_gettid));
    char tmp;
    
    
    read(w->go_fd, &tmp, 1);
    char *mem = malloc(ALLOC_SIZE);
    if (!mem) {
        perror("malloc");
        return 1;
    }
    printf("Worker %d allocated memory, pid=%d, tid=%ld, addr=%p\n", w->id, getpid(), syscall(SYS_gettid), mem);
    write(w->done_fd, "x", 1);


    read(w->go_fd, &tmp, 1);
    for (size_t i = 0; i < ALLOC_SIZE/2; i++) {
        mem[i] = (char)i;
    }
    printf("Worker %d filled first half of memory, pid=%d, tid=%ld, addr=%p\n", w->id, getpid(), syscall(SYS_gettid), mem);
    write(w->done_fd, "x", 1);


    read(w->go_fd, &tmp, 1);
    for (size_t i = ALLOC_SIZE/2; i < ALLOC_SIZE; i++) {
        mem[i] = (char)i;
    }
    printf("Worker %d filled second half of memory, pid=%d, tid=%ld, addr=%p\n", w->id, getpid(), syscall(SYS_gettid), mem);
    write(w->done_fd, "x", 1);


    read(w->go_fd, &tmp, 1);
    free(mem);
    printf("Worker %d freed memory, pid=%d, tid=%ld\n", w->id, getpid(), syscall(SYS_gettid));
    write(w->done_fd, "x", 1);


    return 0;
}

int main(int argc, char **argv) {
    int pipefd[WORKERS][2];
    int donefd[WORKERS][2];

    void *stacks[WORKERS];

    const char *clone_mode = (argc > 1) ? argv[1] : "no_sharing";
    const char *step_mode = (argc > 2) ? argv[2] : "sleep";

    int clone_flags = clone_flags_for_mode(clone_mode);

    char tmp;

    char start_message[128];
    snprintf(start_message, sizeof(start_message), "Start workers in mode %s", clone_mode);
    wait_for_next_step(step_mode, start_message);

    for (int i = 0; i < WORKERS; i++) {
    
        pipe(pipefd[i]);
        pipe(donefd[i]);

        stacks[i] = mmap(
            //kernel chooses the address, so we can pass NULL
            NULL, 
            //size of the stack
            STACK_SIZE,
            //read and write permissions
            PROT_READ | PROT_WRITE,
            //private and anonymous mapping       
            MAP_PRIVATE | MAP_ANONYMOUS,
            //file descriptor, not used for anonymous mapping            
            -1, 
            //offset, not used for anonymous mapping
            0);

        if (stacks[i] == MAP_FAILED) {
            perror("mmap");
            return 1;
        }

        struct worker_arg *arg = malloc(sizeof(*arg));
        arg->id = i;
        arg->go_fd = pipefd[i][0];
        arg->done_fd = donefd[i][1];

        // The stack grows downwards, so we need to pass the top of the stack to clone
        void *stack_top = stacks[i] + STACK_SIZE;

        pid_t pid = clone(
            //worker function
            worker_fn,
            //top of the stack             
            stack_top,
            //flags: share memory space and receive SIGCHLD on exit             
            clone_flags,
            //argument to the worker function              
            arg);

        if (pid == -1) {
            perror("clone");
            return 1;
        }
    }

    printf("Main process (pid=%d, tid=%ld)\n", getpid(), syscall(SYS_gettid));
    usleep(500 * 1000); // 0.5 seconds
    wait_for_next_step(step_mode, "Proceed to malloc");

        for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
        read(donefd[i][0], &tmp, 1);
    }

    usleep(500 * 1000); // 0.5 seconds
    wait_for_next_step(step_mode, "Proceed to fill first half of memory");

    for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
        read(donefd[i][0], &tmp, 1);
    }

    usleep(500 * 1000); // 0.5 seconds
    wait_for_next_step(step_mode, "Proceed to fill second half of memory");

    for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
        read(donefd[i][0], &tmp, 1);
    }
    
    usleep(500 * 1000); // 0.5 seconds
    wait_for_next_step(step_mode, "Proceed to free memory");

    for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
        read(donefd[i][0], &tmp, 1);
    }

    for (int i = 0; i < WORKERS; i++) {
        wait(NULL);
    }

    usleep(500 * 1000); // 0.5 seconds
    wait_for_next_step(step_mode, "Proceed to close program");

    printf("Done\n");
    return 0;
}