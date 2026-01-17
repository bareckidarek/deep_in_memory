package main

import (
    "bytes"
    "encoding/gob"
    "encoding/json"
    "fmt"
    "log"
    "os"

    "go.etcd.io/bbolt"
    // Zaimportuj swój pakiet z ProcessData, np.:
    // "github.com/twoj_user/twoj_repo/internal/data"
    "deep_in_memory/internal/data"
)

func main() {
    db, err := bbolt.Open("../../deep_in_memory.db", 0600, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    result := make(map[string]map[string]interface{})

    err = db.View(func(tx *bbolt.Tx) error {
        return tx.ForEach(func(bucketName []byte, b *bbolt.Bucket) error {
            bucketData := make(map[string]interface{})
            b.ForEach(func(k, v []byte) error {
                // Odszyfruj ProcessData z binariów
                var pd data.ProcessData
                dec := gob.NewDecoder(bytes.NewReader(v))
                if err := dec.Decode(&pd); err != nil {
                    // Jeśli nie uda się zdekodować, pokaż surową wartość jako string
                    bucketData[string(k)] = fmt.Sprintf("decode error: %v", err)
                } else {
                    bucketData[string(k)] = pd
                }
                return nil
            })
            result[string(bucketName)] = bucketData
            return nil
        })
    })
    if err != nil {
        log.Fatal(err)
    }

    enc := json.NewEncoder(os.Stdout)
    enc.SetIndent("", "  ")
    if err := enc.Encode(result); err != nil {
        log.Fatal(err)
    }
}