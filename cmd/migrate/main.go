package main

import (
    "context"
    "log"
    "os"

    "example.com/geocoder/internal/db"
)

func main() {
    url := os.Getenv("DATABASE_URL")
    if url == "" { url = "postgres://geocoder:geocoder@localhost:5432/geocoder?sslmode=disable" }

    pool, err := db.Open(context.Background(), url)
    if err != nil { log.Fatal(err) }
    defer pool.Close()

    data, err := os.ReadFile("migrations/001_init.sql")
    if err != nil { log.Fatal(err) }
    if _, err := pool.Exec(context.Background(), string(data)); err != nil { log.Fatal(err) }
    log.Println("migration complete")
}
