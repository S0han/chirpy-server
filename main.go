package main

import (
	"chirpy-server/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
}

type UserResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
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
	if apiCfg.platform != "dev" {
		respondWithError(w, 403, "403 Forbidden")
		return
	}

	if err := apiCfg.queries.DeleteAllUsers(r.Context()); err != nil {
		respondWithError(w, http.StatusInternalServerError, "database error")
		return
	}

	apiCfg.fileserverHits.Store(0)
	respondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (apiCfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type createUserParams struct {
		Email string `json:"email"`
	}
	var p createUserParams
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	u, err := apiCfg.queries.CreateUser(r.Context(), p.Email)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "database error")
		return
	}

	resp := UserResponse{
		ID:        u.ID.String(),
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Email:     u.Email,
	}

	respondWithJSON(w, http.StatusCreated, resp)
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("warning: .env not loaded:", err)
	}

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	platform := os.Getenv("PLATFORM")

	dbQueries := database.New(db)

	mux := http.NewServeMux()

	apiCfg := &apiConfig{
		queries:  dbQueries,
		platform: platform,
	}

	// /healthz endpoint
	mux.Handle("GET /api/healthz", http.HandlerFunc(handlerHealthz))
	// /metrics endpoint
	mux.Handle("GET /admin/metrics", http.HandlerFunc(apiCfg.handlerMetrics))

	// /validate_chirp endpoint
	mux.Handle("POST /api/validate_chirp", http.HandlerFunc(handlerValidateChirp))
	// /reset endpoint
	mux.Handle("POST /admin/reset", http.HandlerFunc(apiCfg.handlerReset))
	// /create user
	mux.Handle("POST /api/users", http.HandlerFunc(apiCfg.handlerCreateUser))

	// fileserver at /app/
	fs := http.FileServer(http.Dir("."))
	mux.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetricsInc(fs)))

	s := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Fatal(s.ListenAndServe())
}
