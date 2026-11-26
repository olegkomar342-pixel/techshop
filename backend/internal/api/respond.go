package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// writeJSON жауапты JSON түрінде жібереді.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}

// writeError қателердің бірыңғай форматы: { "error": "..." }.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError ішкі қатені логқа жазып, клиентке жалпы хабар қайтарады
// (SQL мәтіні немесе ішкі мәліметтер клиентке көрсетілмейді).
func serverError(w http.ResponseWriter, err error) {
	log.Printf("ERROR: %v", err)
	writeError(w, http.StatusInternalServerError, "сервердің ішкі қатесі")
}

const maxBodyBytes = 1 << 20 // 1 МБ

// decodeJSON сұраныс денесін құрылымға оқиды. Қате болса 400 қайтарып, false береді.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			writeError(w, http.StatusBadRequest, "сұраныс денесі бос")
		case errors.As(err, &maxErr):
			writeError(w, http.StatusBadRequest, "сұраныс денесі тым үлкен")
		default:
			writeError(w, http.StatusBadRequest, "JSON форматы қате")
		}
		return false
	}
	return true
}

// pathID URL-дегі {name} параметрін оң бүтін санға айналдырады.
func pathID(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue(name))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id қате")
		return 0, false
	}
	return id, true
}

// ---------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Logger әр сұранысты логқа жазады.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s → %d", r.Method, r.URL.RequestURI(), rec.status)
		}
	})
}

// Recoverer panic болса серверді құлатпай, 500 қайтарады.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("PANIC: %v", v)
				writeError(w, http.StatusInternalServerError, "сервердің ішкі қатесі")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS фронтенд басқа доменнен ашылғанда керек. origin бос болса — ештеңе қоспайды.
func CORS(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Add("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
