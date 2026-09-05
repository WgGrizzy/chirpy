package main

import (
	"Chirpy/internal/auth"
	"Chirpy/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
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
	dbQueries      *database.Queries
	platform       string
	keySecret      string
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) middlewareMetricsPrint() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Contet-Type", "text/html; charset=utf-8")
		w.Write(fmt.Appendf(nil,
			`<html>
				<body>
					<h1>Welcome, Chirpy Admin</h1>
					<p>Chirpy has been visited %d times!</p>
				</body>
			</html>`,
			cfg.fileserverHits.Load()))
	})
}

func (cfg *apiConfig) DBReset() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//cfg.fileserverHits.Store(0)
		if cfg.platform != "dev" {
			respondWithError(w, 403, "Forbidden")
			return
		}
		err := cfg.dbQueries.DeleteAllUsers(r.Context())
		if err != nil {
			respondWithError(w, 500, "Error Deleting Users")
		}
	})
}

func (cfg *apiConfig) DBCreateUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type User struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Email     string    `json:"email"`
		}
		type NewUserInfo struct {
			Address  string `json:"email"`
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(r.Body)
		newUser := NewUserInfo{}
		err := decoder.Decode(&newUser)
		if err != nil {
			respondWithError(w, 500, "Error Decoding JSON")
			return
		}

		hashed_pass, err := auth.HashPassword(newUser.Password)
		if err != nil {
			respondWithError(w, 500, "Error Hashing Password")
			return
		}

		params := database.CreateUserParams{
			Email:          newUser.Address,
			HashedPassword: hashed_pass,
		}

		dbUsr, err := cfg.dbQueries.CreateUser(r.Context(), params)
		if err != nil {
			respondWithError(w, 500, "Cannot Create User")
			return
		}

		usr := User{
			ID:        dbUsr.ID,
			CreatedAt: dbUsr.CreatedAt,
			UpdatedAt: dbUsr.UpdatedAt,
			Email:     dbUsr.Email,
		}

		respondWithJSON(w, 201, usr)

	})
}

func (cfg *apiConfig) DBUpdateUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type NewUserInfo struct {
			Password string `json:"password"`
			Email    string `json:"email"`
		}
		type User struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Email     string    `json:"email"`
		}

		access, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, 401, "Error Getting Token")
		}
		isValid, err := auth.ValidateJWT(access, cfg.keySecret)
		if err != nil || isValid == uuid.Nil {
			respondWithError(w, 401, "Unauthorized")
		}

		decoder := json.NewDecoder(r.Body)
		newInfo := NewUserInfo{}
		err = decoder.Decode(&newInfo)
		if err != nil {
			respondWithError(w, 500, "Error Decoding JSON")
			return
		}

		hashed_pass, err := auth.HashPassword(newInfo.Password)
		if err != nil {
			respondWithError(w, 500, "Error Hashing Password")
			return
		}

		params := database.UpdateUserInfoParams{
			Email:          newInfo.Email,
			HashedPassword: hashed_pass,
			ID:             isValid,
		}

		updatedUser, err := cfg.dbQueries.UpdateUserInfo(r.Context(), params)
		if err != nil {
			respondWithError(w, 401, "Bad Token")
		}

		user := User{
			ID:        updatedUser.ID,
			CreatedAt: updatedUser.CreatedAt,
			UpdatedAt: updatedUser.UpdatedAt,
			Email:     updatedUser.Email,
		}
		respondWithJSON(w, 200, user)
	})
}

func (cfg *apiConfig) DBLogin() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type LoginInfo struct {
			Password string `json:"password"`
			Email    string `json:"email"`
		}
		type User struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Email     string    `json:"email"`
			Token     string    `json:"token"`
			Refresh   string    `json:"refresh_token"`
		}

		decoder := json.NewDecoder(r.Body)
		login := LoginInfo{}
		err := decoder.Decode(&login)
		if err != nil {
			respondWithError(w, 500, "Error Decoding JSON")
			return
		}

		userByEmail, err := cfg.dbQueries.GetUserByEmail(r.Context(), login.Email)
		if err != nil {
			respondWithError(w, 401, "Unauthorized")
			return
		}

		passMatch, err := auth.CheckPasswordHash(login.Password, userByEmail.HashedPassword)
		if !passMatch || err != nil {
			respondWithError(w, 401, "Unauthroized")
			return
		}

		authToken, err := auth.MakeJWT(userByEmail.ID, cfg.keySecret, 1*time.Hour)
		if err != nil {
			respondWithError(w, 500, "Error Authorizing")
		}

		refresh := auth.MakeRefreshToken()
		refreshTokenParams := database.CreateRefreshTokenParams{
			Token:  refresh,
			UserID: userByEmail.ID,
		}
		err = cfg.dbQueries.CreateRefreshToken(r.Context(), refreshTokenParams)
		if err != nil {
			respondWithError(w, 500, "Error Making Refresh Token")
		}

		returnUser := User{
			ID:        userByEmail.ID,
			CreatedAt: userByEmail.CreatedAt,
			UpdatedAt: userByEmail.UpdatedAt,
			Email:     userByEmail.Email,
			Token:     authToken,
			Refresh:   refresh,
		}
		respondWithJSON(w, 200, returnUser)
	})
}

func (cfg *apiConfig) DBRefreshAccess() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type Access struct {
			Token string `json:"token"`
		}

		headers := r.Header
		refresh := strings.Split(headers.Get("Authorization"), " ")[1]

		tokenInfo, err := cfg.dbQueries.GetInfoFromRefreshToken(r.Context(), refresh)
		if err != nil {
			respondWithError(w, 401, "Token Not Found")
			return
		}

		if time.Now().Compare(tokenInfo.ExpiresAt) > 0 || tokenInfo.RevokedAt.Valid {
			respondWithError(w, 401, "Token Expired or Revoked")
			return
		}

		authToken, err := auth.MakeJWT(tokenInfo.UserID, cfg.keySecret, 1*time.Hour)
		access := Access{
			Token: authToken,
		}
		respondWithJSON(w, 200, access)

	})
}

func (cfg *apiConfig) DBRevokeAccess() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header
		refresh := strings.Split(headers.Get("Authorization"), " ")[1]

		err := cfg.dbQueries.RevokeToken(r.Context(), refresh)
		if err != nil {
			respondWithError(w, 500, "Error Revoking Token")
			return
		}
		w.WriteHeader(204)
	})
}

func (cfg *apiConfig) DBCreateChirp() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type params struct {
			Body   string    `json:"body"`
			UserID uuid.UUID `json:"user_id"`
		}

		authToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			respondWithError(w, 401, "Unauthorized")
			return
		}
		isValid, err := auth.ValidateJWT(authToken, cfg.keySecret)
		if err != nil || isValid == uuid.Nil {
			respondWithError(w, 401, "Unauthorized")
			return
		}

		decoder := json.NewDecoder(r.Body)
		chirpParams := params{}
		err = decoder.Decode(&chirpParams)
		if err != nil {
			respondWithError(w, 500, "Error Decoding JSON")
			return
		}
		if len(chirpParams.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
			return
		}

		bodySlice := strings.Split(chirpParams.Body, " ")
		cleanBody := ""
		for _, val := range bodySlice {
			if cleanBody != "" {
				cleanBody += " "
			}

			lowVal := strings.ToLower(val)
			if lowVal == "kerfuffle" || lowVal == "sharbert" || lowVal == "fornax" {
				cleanBody += "****"
				continue
			}
			cleanBody += val

		}

		newChirpParams := database.CreateChirpParams{
			Body:   cleanBody,
			UserID: isValid,
		}
		newChirp, err := cfg.dbQueries.CreateChirp(r.Context(), newChirpParams)
		if err != nil {
			respondWithError(w, 500, "Cannot Create Chirp")
			return
		}

		chirp := Chirp{
			ID:        newChirp.ID,
			CreatedAt: newChirp.CreatedAt,
			UpdatedAt: newChirp.UpdatedAt,
			Body:      newChirp.Body,
			UserID:    newChirp.UserID,
		}

		respondWithJSON(w, 201, chirp)

	})
}

func (cfg *apiConfig) DBGetAllChirps() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allChirps, err := cfg.dbQueries.GetAllChirps(r.Context())
		if err != nil {
			respondWithError(w, 500, "Error Getting Chirps")
			return
		}

		returnChirps := []Chirp{}
		for _, chirp := range allChirps {
			newChirp := Chirp{
				ID:        chirp.ID,
				CreatedAt: chirp.CreatedAt,
				UpdatedAt: chirp.UpdatedAt,
				Body:      chirp.Body,
				UserID:    chirp.UserID,
			}
			returnChirps = append(returnChirps, newChirp)
		}

		respondWithJSON(w, 200, returnChirps)
	})
}

func (cfg *apiConfig) DBGetChirpByID() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		searchVal, err := cfg.dbQueries.GetChirpByID(r.Context(), id)
		if err != nil {
			respondWithError(w, 404, "No Chirp Found")
			return
		}

		chirp := Chirp{
			ID:        searchVal.ID,
			CreatedAt: searchVal.CreatedAt,
			UpdatedAt: searchVal.UpdatedAt,
			Body:      searchVal.Body,
			UserID:    searchVal.UserID,
		}

		respondWithJSON(w, 200, chirp)
	})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type returnErr struct {
		Msg string `json:"error"`
	}
	respBody := returnErr{Msg: msg}

	data, err := json.Marshal(respBody)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)

}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	keySecret := os.Getenv("KEY_SECRET")
	db, _ := sql.Open("postgres", dbURL)
	dbQueries := database.New(db)

	mux := http.NewServeMux()
	ser := http.Server{}
	ser.Handler = mux
	ser.Addr = ":8080"

	h1 := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Contet-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write(([]byte)("OK"))
	}
	mux.HandleFunc("GET /api/healthz", h1)

	apiCfg := apiConfig{}
	apiCfg.dbQueries = dbQueries
	apiCfg.platform = platform
	apiCfg.keySecret = keySecret
	fileSer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileSer))

	mux.Handle("GET /admin/metrics", apiCfg.middlewareMetricsPrint())
	mux.Handle("POST /admin/reset", apiCfg.DBReset())
	mux.Handle("POST /api/users", apiCfg.DBCreateUser())
	mux.Handle("PUT /api/users", apiCfg.DBUpdateUser())
	mux.Handle("POST /api/chirps", apiCfg.DBCreateChirp())

	mux.Handle("GET /api/chirps/{id}", apiCfg.DBGetChirpByID())
	mux.Handle("GET /api/chirps", apiCfg.DBGetAllChirps())
	mux.Handle("POST /api/login", apiCfg.DBLogin())
	mux.Handle("POST /api/refresh", apiCfg.DBRefreshAccess())
	mux.Handle("POST /api/revoke", apiCfg.DBRevokeAccess())

	ser.ListenAndServe()
}
