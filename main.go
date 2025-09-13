package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	respondWithJSON(w, http.StatusOK, []byte("Hits have been reset to 0"))
}

func handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	if len(params.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	profanities := [3]string{"kerfuffle", "sharbert", "fornax"}
	split_body := strings.Split(params.Body, " ")
	cleaned_body_split := []string{}

	for i := 0; i < len(split_body); i++ {
		temp := split_body[i]
		for j := 0; j < len(profanities); j++ {
			if strings.ToLower(temp) == profanities[j] {
				temp = "****"
				break
			}
		}
		cleaned_body_split = append(cleaned_body_split, temp)
	}

	cleaned_body := strings.Join(cleaned_body_split, " ")

	respondWithJSON(w, http.StatusOK, map[string]string{"cleaned_body": cleaned_body})
}

func handlerHealthz(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, []byte("OK"))
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func main() {
	mux := http.NewServeMux()

	apiCfg := &apiConfig{}

	// /healthz endpoint
	mux.Handle("GET /api/healthz", http.HandlerFunc(handlerHealthz))
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
