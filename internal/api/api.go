package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"example.com/geocoder/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type API struct{ DB *pgxpool.Pool }

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/v1/search", a.search)
	mux.HandleFunc("/v1/reverse", a.reverse)
	mux.HandleFunc("/v1/autocomplete", a.autocomplete)
	return mux
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.DB.Ping(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "q is required", 400)
		return
	}
	limit := intParam(r, "limit", 10, 1, 100)

	var bbox *[4]float64
	if s := r.URL.Query().Get("bbox"); s != "" {
		p := strings.Split(s, ",")
		if len(p) != 4 {
			http.Error(w, "bbox must be minLon,minLat,maxLon,maxLat", 400)
			return
		}
		var b [4]float64
		for i := range b {
			v, err := strconv.ParseFloat(strings.TrimSpace(p[i]), 64)
			if err != nil {
				http.Error(w, "invalid bbox", 400)
				return
			}
			b[i] = v
		}
		if b[0] < -180 || b[2] > 180 || b[1] < -90 || b[3] > 90 || b[0] > b[2] || b[1] > b[3] {
			http.Error(w, "invalid bbox bounds", 400)
			return
		}
		bbox = &b
	}

	var lat, lon *float64
	if r.URL.Query().Get("lat") != "" || r.URL.Query().Get("lon") != "" {
		la, e1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
		lo, e2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
		if e1 != nil || e2 != nil || la < -90 || la > 90 || lo < -180 || lo > 180 {
			http.Error(w, "lat/lon must be valid", 400)
			return
		}
		lat, lon = &la, &lo
	}

	results, err := db.Search(r.Context(), a.DB, q, bbox, lat, lon, floatParam(r, "radius", 0), limit)
	if err != nil {
		log.Println(err)
		http.Error(w, "search failed", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"results": results})
}

func (a *API) reverse(w http.ResponseWriter, r *http.Request) {
	lat, e1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, e2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if e1 != nil || e2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		http.Error(w, "valid lat/lon are required", 400)
		return
	}
	result, err := db.Reverse(r.Context(), a.DB, lat, lon, floatParam(r, "radius", 1000))
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 200, map[string]any{"result": nil})
		return
	}
	if err != nil {
		log.Println(err)
		http.Error(w, "reverse geocode failed", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"result": result})
}

func (a *API) autocomplete(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "q is required", 400)
		return
	}
	results, err := db.Autocomplete(r.Context(), a.DB, q, intParam(r, "limit", 10, 1, 50))
	if err != nil {
		log.Println(err)
		http.Error(w, "autocomplete failed", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"results": results})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func intParam(r *http.Request, key string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || v < min || v > max {
		return def
	}
	return v
}

func floatParam(r *http.Request, key string, def float64) float64 {
	v, err := strconv.ParseFloat(r.URL.Query().Get(key), 64)
	if err != nil || v < 0 {
		return def
	}
	return v
}
