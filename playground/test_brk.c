#include <stdio.h>
#include <unistd.h>
#include <sys/syscall.h>

int main() {
    void *current_brk, *new_brk;
    
    printf("Testing brk syscall...\n");
    
    // Get current program break
    current_brk = (void*)syscall(SYS_brk, 0);
    printf("Initial program break: %p\n", current_brk);
    
    // Wait a bit
    sleep(2);
    
    // Increase program break by 4096 bytes (1 page)
    new_brk = (void*)syscall(SYS_brk, (char*)current_brk + 4096);
    printf("After extending by 4096 bytes: %p\n", new_brk);
    
    // Wait a bit
    sleep(2);
    
    // Increase program break by another 8192 bytes (2 pages)
    new_brk = (void*)syscall(SYS_brk, (char*)new_brk + 8192);
    printf("After extending by 8192 bytes: %p\n", new_brk);
    
    // Wait a bit
    sleep(2);
    
    // Shrink program break
    new_brk = (void*)syscall(SYS_brk, (char*)current_brk + 2048);
    printf("After shrinking to +2048 bytes: %p\n", new_brk);
    
    // Wait before exit
    sleep(5);
    
    printf("Done!\n");
    return 0;
}
