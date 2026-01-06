#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <malloc.h>

int main(void) {
    //malloc_info(0, stdout);
    
    void *a;
    int counter = 1;
    while (1) {
        //char c;
        //read(0, &c, 1);
        
        //int liczba = rand() % 100 + 1;
        void *b;
        // if(liczba % 2 == 0) {
        //     b = malloc(sizeof(bool));
        // } else {
        //     b = malloc(1024 * 1024 * 10);
        // }

        //b = malloc(sizeof(bool));
        //b = malloc(1024  * 10);
        b = malloc(1024 * 1024);

        if (!b) {
            perror("malloc");
            return 1;
        }

        //read(0, &c, 1);
        sleep(2);

        for(int i = 0; i < 1024 * 1024; i++) {
            ((char *)b)[i] = 'a';
        }
        //strcpy(b, "ala ma kota");

        if(counter == 0) {
            a = b;
        }

        // if(counter == 4) {
        //     printf("First allocated block content: %.12s\n", (char *)a);
        //     for(int i = 0; i < 1024 * 512; i++) {
        //         printf("%c", ((char *)a)[i]);
        //     }
        // }

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