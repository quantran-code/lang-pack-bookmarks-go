package main

import (
	"log"
	"net/http"
	"os"

	"github.com/quantran-code/lang-pack-bookmarks-go/internal/database"
	"github.com/quantran-code/lang-pack-bookmarks-go/internal/httpapi"
)

func main() {
	dsn := os.Getenv("DB_PATH")
	if dsn == "" {
		dsn = database.MemoryDSN
	}
	db, err := database.Open(dsn)
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := httpapi.ConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, httpapi.NewRouter(db, cfg)))
}
