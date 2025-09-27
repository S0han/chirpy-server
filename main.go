package main

import (
	"chirpy-server/internal/database"
	"chirpy-server/internal/auth"
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
	queries        	*database.Queries
	platform       	string
	JWTSecret	   	string
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
		Email 		string 	`json:"email"`
		Password 	string 	`json:"password"`
	}
	var p createUserParams
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	hp, err := auth.HashPassword(p.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not hash password")
    	return
	}

	u, err := apiCfg.queries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          p.Email,
		HashedPassword: hp,
	})
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

func (apiCfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	type updateUserParams struct {
		Email 		string 	`json:"email"`
		Password 	string	`json:"password"` 
	}
	var p updateUserParams
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
	}

	tokenStr, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	uid, err := auth.ValidateJWT(tokenStr, apiCfg.JWTSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	hp, err := auth.HashPassword(p.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not hash password")
    	return
	}

	u, err := apiCfg.queries.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:				uid,
		Email:          p.Email,
		HashedPassword: hp,
	})
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
	respondWithJSON(w, http.StatusOK, resp)
}

func (apiCfg *apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	chirpIDStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDStr)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "id not found")
		return 
	}
	
	ch, err := apiCfg.queries.GetChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "chirp not found")
		return 
	}

	resp := ChirpResponse{
        ID:        ch.ID.String(),
        CreatedAt: ch.CreatedAt,
        UpdatedAt: ch.UpdatedAt,
        Body:      ch.Body,
        UserID:    ch.UserID.String(),
    }

	respondWithJSON(w, http.StatusOK, resp)
}

func (apiCfg *apiConfig) handlerGetAllChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := apiCfg.queries.GetAllChirps(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not receive chirps")
		return
	}

	resp := []ChirpResponse{}
	for _, ch := range chirps {
		resp = append(resp, ChirpResponse{
			ID:        ch.ID.String(),
			CreatedAt: ch.CreatedAt,
			UpdatedAt: ch.UpdatedAt,
			Body:      ch.Body,
			UserID:    ch.UserID.String(),
		})
	}

	respondWithJSON(w, http.StatusOK, resp)
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

	tokenStr, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	uid, err := auth.ValidateJWT(tokenStr, apiCfg.JWTSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid token")
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

func (apiCfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	type loginParams struct {
		Email 				string 	`json:"email"`
		Password 			string 	`json:"password"`
	}
	var p loginParams
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	u, err := apiCfg.queries.GetUserByEmail(r.Context(), p.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
        return
	}

	if err := auth.CheckPasswordHash(p.Password, u.HashedPassword); err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
        return
	}

	resp := UserResponse{
		ID:        u.ID.String(),
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Email:     u.Email,
	}

	token, err := auth.MakeJWT(u.ID, apiCfg.JWTSecret, time.Hour*1)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not generate token")
		return
	}

	refreshToken := auth.MakeRefreshToken()

	_, err = apiCfg.queries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    u.ID,
		ExpiresAt: time.Now().UTC().Add(time.Hour * 24 * 60), // 60 days
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not save refresh token")
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"id":            resp.ID,
		"created_at":    resp.CreatedAt,
		"updated_at":    resp.UpdatedAt,
		"email":         resp.Email,
		"token":         token,
		"refresh_token": refreshToken,
	})
}

func (apiCfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	// Get the refresh token from the Authorization header
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	// Get user from refresh token (this also validates the token)
	user, err := apiCfg.queries.GetUserFromRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Create a new access token
	accessToken, err := auth.MakeJWT(user.ID, apiCfg.JWTSecret, time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not generate access token")
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"token": accessToken,
	})
}

func (apiCfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	// Get the refresh token from the Authorization header
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	// Revoke the refresh token
	_, err = apiCfg.queries.RevokeRefreshToken(r.Context(), refreshToken)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not revoke token")
		return
	}

	// Return 204 No Content
	w.WriteHeader(http.StatusNoContent)
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

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET not set")
	}

	apiCfg := &apiConfig{
		queries:  dbQueries,
		platform: platform,
		JWTSecret: secret,
	}

	// /healthz endpoint
	mux.HandleFunc("/api/healthz", handlerHealthz)
	// /metrics endpoint
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	// /retrieve all chirps
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetAllChirps)
	// /retierve specific chirp
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirp)

	// /reset endpoint
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	// /create user
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	// /create chirp
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerCreateChirp)
	// /login endpoint
	mux.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	// /refresh endpoint
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	// /revoke endpoint
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)


	// /update user
	mux.HandleFunc("PUT /api/users", apiCfg.handlerUpdateUser)

	// fileserver at /app/
	fs := http.FileServer(http.Dir("."))
	mux.Handle("/app/", http.StripPrefix("/app/", apiCfg.middlewareMetricsInc(fs)))

	s := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Fatal(s.ListenAndServe())
}
