#define _POSIX_C_SOURCE 200809L
#define _GNU_SOURCE

#include <stdio.h>
#include <stdlib.h>
#include <pthread.h>
#include <unistd.h>
#include <string.h>
#include <sys/syscall.h>
#include <sys/types.h>

#define THREADS 2
#define ALLOC_SIZE 1024 * 1024 // 1 MB

typedef enum {
    STEP_ALLOC,
    STEP_MEMSET,
    STEP_FREE,
    STEP_EXIT
} step_t;

step_t step;

pthread_barrier_t barrier;

void* worker(void* arg) {
    void* memory = NULL;
    long id = (long)arg;

    while (1) {
        // czekamy aż main ustawi step
        pthread_barrier_wait(&barrier);

        if (step == STEP_ALLOC) {
            memory = malloc(ALLOC_SIZE);
            printf("[T%ld, %d, %ld] alloc\n", id, getpid(), syscall(SYS_gettid));
        }
        else if (step == STEP_MEMSET) {
            if (memory) {
                //memset(memory, 0, ALLOC_SIZE);
                for(int i = 0; i < ALLOC_SIZE; i++) {
                    ((char *)memory)[i] = 'a';
                }
                printf("[T%ld, %d, %ld] memset\n", id, getpid(), syscall(SYS_gettid));
            }
        }
        else if (step == STEP_FREE) {
            free(memory);
            memory = NULL;
            printf("[T%ld, %d, %ld] free\n", id, getpid(), syscall(SYS_gettid));
        }
        else if (step == STEP_EXIT) {
            break;
        }

        // koniec etapu — synchronizacja
        pthread_barrier_wait(&barrier);
    }

    return NULL;
}

static void wait_for_next_step(const char *step_mode, const char *message) {
    if (step_mode != NULL && strcmp(step_mode, "manual") == 0) {
        printf("%s (press ENTER)\n", message);
        getchar();
        return;
    }

    printf("%s (waiting 5 seconds)\n", message);
    sleep(5);
}

int main(int argc, char *argv[]) {
    pthread_t threads[THREADS];
    
    const char *mode = (argc > 1) ? argv[1] : NULL;

    wait_for_next_step(mode, "=== Step 0/4: Start multi-threaded allocation scenario ===");

    // +1 bo main też bierze udział
    pthread_barrier_init(&barrier, NULL, THREADS + 1);

    for (long i = 0; i < THREADS; i++) {
        pthread_create(&threads[i], NULL, worker, (void*)i);
        printf("[T%ld, %d, %ld] created\n", i, getpid(), syscall(SYS_gettid));
    }

    wait_for_next_step(mode, "=== Step 1/4: Allocate memory in all threads ===");
    step = STEP_ALLOC;
    pthread_barrier_wait(&barrier); // start etapu
    pthread_barrier_wait(&barrier); // koniec etapu

    wait_for_next_step(mode, "=== Step 2/4: Set memory in all threads ===");
    step = STEP_MEMSET;
    pthread_barrier_wait(&barrier);
    pthread_barrier_wait(&barrier);

    wait_for_next_step(mode, "=== Step 3/4: Free memory in all threads ===");
    step = STEP_FREE;
    pthread_barrier_wait(&barrier);
    pthread_barrier_wait(&barrier);

    wait_for_next_step(mode, "=== Step 4/4: Exit threads ===");
    step = STEP_EXIT;
    pthread_barrier_wait(&barrier); // pozwól workerom wyjść

    for (int i = 0; i < THREADS; i++) {
        pthread_join(threads[i], NULL);
    }

    pthread_barrier_destroy(&barrier);

    return 0;
}