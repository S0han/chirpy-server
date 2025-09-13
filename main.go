package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (apiCfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (apiCfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`
		<html>
			<body>
				<h1>Welcome, Chirpy Admin</h1>
				<p>Chirpy has been visited %d times!</p>
			</body>
		</html>
	`,
		apiCfg.fileserverHits.Load())))
}

func (apiCfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	apiCfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits have been reset to 0"))
}

func handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Something went wrong"})
		return
	}

	if len(params.Body) > 140 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Something went wrong"})
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]bool{"valid": true})
}

func main() {
	mux := http.NewServeMux()

	apiCfg := &apiConfig{}

	// /healthz endpoint
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	// /metrics endpoint
	mux.Handle("GET /admin/metrics", http.HandlerFunc(apiCfg.handlerMetrics))

	// /validate_chirp endpoint
	mux.Handle("POST /api/validate_chirp", http.HandlerFunc(handlerValidateChirp))
	// /reset endpoint
	mux.Handle("POST /admin/reset", http.HandlerFunc(apiCfg.handlerReset))

	// fileserver at /app/
	fs := http.FileServer(http.Dir("."))
	mux.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetricsInc(fs)))

	s := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Fatal(s.ListenAndServe())
}
