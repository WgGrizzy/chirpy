package main
import ("net/http"; "sync/atomic"; "fmt"; "encoding/json"; "strings"; _ "github.com/lib/pq")

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w,r)
	})
}

func (cfg *apiConfig) middlewareMetricsPrint() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Contet-Type", "text/html; charset=utf-8")
		w.Write(([]byte)(fmt.Sprintf(
			"<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())))
	})
}

func (cfg *apiConfig) middlewareMetricsReset() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Store(0)
	})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}){
	data, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func respondWithError(w http.ResponseWriter, code int, msg string){
	type returnErr struct {
		Msg string `json:"error"`
	}
	respBody := returnErr{ Msg: msg, }

	data, err := json.Marshal(respBody)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)


}

func main(){
	mux := http.NewServeMux()
	ser := http.Server{}
	ser.Handler = mux
	ser.Addr = ":8080"


	h1 := func(w http.ResponseWriter, _ *http.Request){
		w.Header().Set("Contet-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write(([]byte)("OK"))
	}
	mux.HandleFunc("GET /api/healthz", h1)

	validator := func(w http.ResponseWriter, r *http.Request){
		type chirp struct {
			Body string `json:"body"`
		}
		type returnVals struct {
			Clean string `json:"cleaned_body"`
		}

		decoder := json.NewDecoder(r.Body)
		chirpText := chirp{}
		err := decoder.Decode(&chirpText)
		if err != nil {
			respondWithError(w, 500, "Error Decoding JSON")
			return
		}
		if len(chirpText.Body) > 140{
			respondWithError(w, 400, "Chirp is too long")
			return
		}
		bodySlice := strings.Split(chirpText.Body, " ")
		cleanBody := ""
		for _, val := range bodySlice{
			if cleanBody != ""{
				cleanBody += " "
			}

			lowVal := strings.ToLower(val)
			if lowVal == "kerfuffle" || lowVal == "sharbert" || lowVal == "fornax"{
				cleanBody += "****"
				continue
			}
			cleanBody += val
		
		}

		respBody := returnVals { Clean: cleanBody, }
		respondWithJSON(w, 200, respBody)
	}
	mux.HandleFunc("POST /api/validate_chirp", validator)


	apiCfg := apiConfig{}
	fileSer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileSer))

	mux.Handle("GET /admin/metrics", apiCfg.middlewareMetricsPrint())
	mux.Handle("POST /admin/reset", apiCfg.middlewareMetricsReset())
	
	
	ser.ListenAndServe()
}
