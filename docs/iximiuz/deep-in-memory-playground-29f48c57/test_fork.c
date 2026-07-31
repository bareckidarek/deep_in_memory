#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/mman.h>
#include <sys/syscall.h>
#include <sys/wait.h>

#define ALLOC_SIZE (200 * 1024)
#define WORKERS 10

struct worker_arg {
    int id;
    int go_fd;
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

static int worker_fn(struct worker_arg *w) {
    printf("Worker %d ready (pid=%d, tid=%ld)\n", w->id, getpid(), syscall(SYS_gettid));

    char tmp;
    if (read(w->go_fd, &tmp, 1) != 1) {
        perror("read");
        return 1;
    }

    char *mem = malloc(ALLOC_SIZE);
    if (!mem) {
        perror("malloc");
        return 1;
    }

    printf("Worker %d allocated memory, pid=%d, tid=%ld, addr=%p\n", w->id, getpid(), syscall(SYS_gettid), mem);

    if (read(w->go_fd, &tmp, 1) != 1) {
        perror("read");
        free(mem);
        return 1;
    }

    for (size_t i = 0; i < ALLOC_SIZE; i++) {
        mem[i] = (char)i;
    }

    printf("Worker %d filled memory, pid=%d, tid=%ld, addr=%p\n", w->id, getpid(), syscall(SYS_gettid), mem);

    if (read(w->go_fd, &tmp, 1) != 1) {
        perror("read");
        free(mem);
        return 1;
    }

    free(mem);

    printf("Worker %d freed memory, pid=%d, tid=%ld\n", w->id, getpid(), syscall(SYS_gettid));

    if (read(w->go_fd, &tmp, 1) != 1) {
        perror("read");
        return 1;
    }

    printf("Worker %d exiting, pid=%d, tid=%ld\n", w->id, getpid(), syscall(SYS_gettid));

    return 0;
}

int main(int argc, char **argv) {
    int pipefd[WORKERS][2] = {0};
    pid_t pids[WORKERS];
    const char *step_mode = (argc > 1) ? argv[1] : NULL;

    for (int i = 0; i < WORKERS; i++) {
        pipefd[i][0] = -1;
        pipefd[i][1] = -1;
    }

    wait_for_next_step(step_mode, "Start workers");

    for (int i = 0; i < WORKERS; i++) {
        if (pipe(pipefd[i]) == -1) {
            perror("pipe");
            return 1;
        }

        pid_t pid = fork();
        if (pid == -1) {
            perror("fork");
            return 1;
        }

        if (pid == 0) {
            struct worker_arg arg = {
                .id = i,
                .go_fd = pipefd[i][0],
            };

            close(pipefd[i][1]);

            for (int j = 0; j < i; j++) {
                if (pipefd[j][0] != -1) {
                    close(pipefd[j][0]);
                }

                if (pipefd[j][1] != -1) {
                    close(pipefd[j][1]);
                }
            }

            _exit(worker_fn(&arg));
        }

        pids[i] = pid;
        close(pipefd[i][0]);
        pipefd[i][0] = -1;
    }

    printf("Main process (pid=%d, tid=%ld)\n", getpid(), syscall(SYS_gettid));
    usleep(500 * 1000);
    wait_for_next_step(step_mode, "Proceed to malloc");

    for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
    }

    usleep(500 * 1000);
    wait_for_next_step(step_mode, "Proceed to fill memory");

    for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
    }

    usleep(500 * 1000);
    wait_for_next_step(step_mode, "Proceed to free memory");

    for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
    }

    usleep(500 * 1000);
    wait_for_next_step(step_mode, "Proceed to exit workers");
    for (int i = 0; i < WORKERS; i++) {
        write(pipefd[i][1], "x", 1);
        close(pipefd[i][1]);
    }

    for (int i = 0; i < WORKERS; i++) {
        waitpid(pids[i], NULL, 0);
    }

    printf("Done\n");
    return 0;
}