#define _GNU_SOURCE

#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

#define SIZE (100 * 1024)
#define STACK_SIZE (1024 * 1024)

static char child_stack[STACK_SIZE];
static const char *g_step_mode;

static void wait_for_next_step(const char *step_mode, const char *message)
{
    if (step_mode != NULL && strcmp(step_mode, "manual") == 0) {
        printf("%s (press ENTER)\n", message);
        getchar();
        return;
    }

    printf("%s (waiting 5 seconds)\n", message);
    sleep(5);
}

static int child_fn(void *arg)
{
    char *buf = arg;

    printf("[child] pid=%d\n", getpid());

    wait_for_next_step(g_step_mode, "[child] Trigger COW");

    memset(buf, 'C', SIZE);

    printf("[child] memset done. Inspect pfn addresses.\n");

    wait_for_next_step(g_step_mode, "[child] Close child");

    return 0;
}

int main(int argc, char **argv)
{
    g_step_mode = (argc > 1) ? argv[1] : NULL;

    printf("[parent] pid=%d\n", getpid());

    wait_for_next_step(g_step_mode, "[parent] Allocate memory");

    char *buf = malloc(SIZE);
    if (!buf) {
        perror("malloc");
        return 1;
    }

    wait_for_next_step(g_step_mode, "[parent] Fill memory with 'P'");

    memset(buf, 'P', SIZE);

    printf("[parent] memset done.\n");

    wait_for_next_step(g_step_mode, "[parent] Clone child");

    pid_t child = clone(
        child_fn,
        child_stack + STACK_SIZE,
        SIGCHLD,
        buf);

    if (child == -1) {
        perror("clone");
        return 1;
    }

    printf("[parent] child pid=%d, Inspect pfn addresses.\n", child);

    waitpid(child, NULL, 0);

    free(buf);
    return 0;
}