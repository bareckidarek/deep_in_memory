#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <malloc.h>

#define ALLOC_SIZE 1024 * 100 //brk


//#define ALLOC_SIZE 1024 * 1024 * 2 //mmap

int main(void) {
    //malloc_info(0, stdout);
    
    void *a;
    int counter = 0;
    //char c;
    //read(0, &c, 1);
    while (1) {
        char c;
        read(0, &c, 1);

        //int liczba = rand() % 100 + 1;
        void *b;
        // if(liczba % 2 == 0) {
        //     b = malloc(sizeof(bool));
        // } else {
        //     b = malloc(1024 * 1024 * 10);
        // }

        //b = malloc(sizeof(bool));
        //b = malloc(1024  * 10);
        b = malloc(ALLOC_SIZE);

        if (!b) {
            perror("malloc");
            return 1;
        }

        read(0, &c, 1);
        //sleep(2);

        for(int i = 0; i < ALLOC_SIZE; i++) {
            //usleep(1 * 10);
            //memset(&((char *)b)[i], 'a', 64);
            ((char *)b)[i] = 'a';
        }
        //strcpy(b, "ala ma kota");

        if(counter == 0) {
            a = b;
        }

        read(0, &c, 1);

        if(counter == 0) {
            printf("First allocated block content: %.12s\n", (char *)a);
            for(int i = 0; i < ALLOC_SIZE; i++) {
                //usleep(1 * 10);
                char d = ((char *)a)[i];
                if (d != 'a') {
                    break;
                }
            }
        }

        // if (counter % 10 == 0)
        // {
        //    free(b);
        //}
        
        //read(0, &c, 1);
        //sleep(5);
        //free(b);
        //malloc_info(0, stdout);
        counter++;
    }
    return 0;
}