package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"imiq/geocode2/internal/api"
	"imiq/geocode2/internal/db"
)

func main() {
	database := flag.String("database", "postgres://geocoder:geocoder@localhost:5432/geocoder?sslmode=disable", "database URL")
	embeddingsURL := flag.String("embeddings-url", "http://ollama:11434/v1/embeddings", "api endpoint for generating embeddings")
	listen := flag.String("listen", ":8080", "listen address")
	flag.Parse()

	pool, err := db.Open(context.Background(), *database)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	handler := cors((&api.API{DB: pool, EmbeddingsURL: *embeddingsURL}).Routes())

	log.Printf("listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, handler))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
