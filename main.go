package main

import (
	"log"
	"net/http"
)

func main() {
	ServeMux := http.NewServeMux()

	s := &http.Server{
		Addr:    ":8080",
		Handler: ServeMux,
	}
	log.Fatal(s.ListenAndServe())
}
