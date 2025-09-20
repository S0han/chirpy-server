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

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
}

type UserResponse struct {
	ID        	string    	`json:"id"`
	CreatedAt 	time.Time 	`json:"created_at"`
	UpdatedAt 	time.Time 	`json:"updated_at"`
	Email     	string    	`json:"email"`
}

type ChirpResponse struct {
	ID 			string 		`json:"id"`
	CreatedAt 	time.Time 	`json:"created_at"`
	UpdatedAt 	time.Time 	`json:"updated_at"`
	Body 		string 		`json:"body"`
	UserID     	string		`json:"user_id"` 		
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
        fmt.Println(err)
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
		fmt.Println(err)
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

func (apiCfg *apiConfig) handlerCreateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body 	string 	`json:"body"`
		UserID 	string 	`json:"user_id"`
	}
	var p parameters
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(p.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	profanities := [3]string{"kerfuffle", "sharbert", "fornax"}
	split_body := strings.Split(p.Body, " ")
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
	cleaned := strings.Join(cleaned_body_split, " ")

	uid, err := uuid.Parse(p.UserID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	ch, err := apiCfg.queries.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleaned,
		UserID: uid,
	})

	resp := ChirpResponse{
        ID:        ch.ID.String(),
        CreatedAt: ch.CreatedAt,
        UpdatedAt: ch.UpdatedAt,
        Body:      ch.Body,
        UserID:    ch.UserID.String(),
    }
    respondWithJSON(w, http.StatusCreated, resp)
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
	mux.HandleFunc("/api/healthz", handlerHealthz)
	// /metrics endpoint
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)

	// /reset endpoint
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	// /create user
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	// /create chirp
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerCreateChirp)

	// fileserver at /app/
	fs := http.FileServer(http.Dir("."))
	mux.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetricsInc(fs)))

	s := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Fatal(s.ListenAndServe())
}
