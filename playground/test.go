package main

import (
    "bufio"
    "fmt"
    "os"
)

func main() {
    fmt.Printf("PID procesu: %d\n", os.Getpid())

    r := bufio.NewReader(os.Stdin)
    for {
        _, err := r.ReadByte()
        if err != nil {
            // EOF lub błąd wejścia
            break
        }

        // Krótkotrwała alokacja bool
        b := new(bool)
        if b == nil {
            fmt.Fprintln(os.Stderr, "malloc failed")
            os.Exit(1)
        }
        // GC zwolni, więc tylko zerujemy referencję
        b = nil
    }
}