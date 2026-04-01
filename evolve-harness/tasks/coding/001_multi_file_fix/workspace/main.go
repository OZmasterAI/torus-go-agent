package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	repo := NewUserRepo()
	handler := NewHandler(repo)

	mux := http.NewServeMux()
	SetupRoutes(mux, handler)

	addr := ":8080"
	fmt.Printf("User service starting on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
