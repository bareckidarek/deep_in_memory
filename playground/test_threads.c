#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <pthread.h>

void* worker(void* arg) {
    while (1) {
        bool *b = malloc(1024 * 1024);
        sleep(1);
        //free(b);
    }
    return NULL;
}

int main(void) {
    while (1) {
        char c;
        read(0, &c, 1);
        
        //bool *b = malloc(sizeof(bool));

        // if (!b) {
        //     perror("malloc");
        //     return 1;
        // }

        pthread_t thread;

        pthread_create(&thread, NULL, worker, NULL);
        //pthread_join(thread, NULL);
        pthread_detach(thread);

        //free(b);
    }
    return 0;
}