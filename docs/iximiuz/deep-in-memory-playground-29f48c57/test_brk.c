#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>

#define ALLOC_SIZE (1024 * 100) //brk

static void wait_for_next_step(const char *step_mode, int counter, int steps, const char *title) {
    char message[256];
    snprintf(message, sizeof(message), "=== Step %d/%d: %s ===", counter, steps, title);

    if (step_mode != NULL && strcmp(step_mode, "manual") == 0) {
        printf("%s (press ENTER)\n", message);
        getchar();
        return;
    }

    printf("%s (waiting 5 seconds)\n", message);
    sleep(5);
}

int main(int argc, char *argv[]) {

    const char *step_mode = (argc > 1) ? argv[1] : NULL;

    printf("Start brk allocation scenario\n");
    
    wait_for_next_step(step_mode, 1, 9, "Allocate first block of memory");
    void *b1 = malloc(ALLOC_SIZE);

    wait_for_next_step(step_mode, 2, 9, "Fill the first half of the allocated block with 'a' characters to trigger page faults and physical page assignment");
    memset(&((char *)b1)[0], 'a', ALLOC_SIZE/2);

    wait_for_next_step(step_mode, 3, 9, "Fill the second half of the allocated block with 'a' characters to trigger more page faults and physical page assignment");
    memset(&((char *)b1)[0+ALLOC_SIZE/2], 'a', ALLOC_SIZE/2);

    wait_for_next_step(step_mode, 4, 9, "Allocate second block of memory to observe how the allocator handles multiple large allocations");
    void *b2 = malloc(ALLOC_SIZE);

    wait_for_next_step(step_mode, 5, 9, "Fill the second block with 'b' characters to trigger page faults and physical page assignment");
    memset(&((char *)b2)[0], 'b', ALLOC_SIZE);

    wait_for_next_step(step_mode, 6, 9, "Read from the first block to observe if it was swapped out under memory pressure and to trigger swap-in if necessary");
    for(int i = 0; i < ALLOC_SIZE; i++) {
        char d = ((char *)b1)[i];
    }

    wait_for_next_step(step_mode, 7, 9, "Free the first block to observe how the allocator releases memory and if it can reclaim the virtual mapping");
    free(b1);

    wait_for_next_step(step_mode, 8, 9, "Free the second block to observe the release path for large mappings and to see if the virtual mapping is also released");
    free(b2);

    wait_for_next_step(step_mode, 9, 9, "End of scenario.");
    return 0;
}
