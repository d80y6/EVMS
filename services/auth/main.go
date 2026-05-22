package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		// Basic auth skeleton
		w.Write([]byte("Login Endpoint"))
	})

	log.Println("Auth Service listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
