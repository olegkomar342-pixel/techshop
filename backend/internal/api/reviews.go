package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type reviewRequest struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

// POST /api/products/{id}/reviews — пікір қалдыру.
// Бір пайдаланушы бір тауарға бір пікір қалдырады; қайта жіберсе — пікірі жаңартылады.
func (s *Server) handleUpsertReview(w http.ResponseWriter, r *http.Request) {
	productID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req reviewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Comment = strings.TrimSpace(req.Comment)
	v := validator{}
	v.check(req.Rating >= 1 && req.Rating <= 5, "rating", "баға 1-ден 5-ке дейін болуы керек")
	v.length("comment", req.Comment, 0, 2000)
	if !v.ok(w) {
		return
	}
	if _, ok := s.activeProductStock(w, r, productID); !ok {
		return
	}

	me := currentUser(r)
	var rv Review
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO reviews (product_id, user_id, rating, comment) VALUES ($1, $2, $3, $4)
		ON CONFLICT (product_id, user_id)
		DO UPDATE SET rating = EXCLUDED.rating, comment = EXCLUDED.comment, created_at = now()
		RETURNING id, product_id, user_id, rating, comment, created_at`,
		productID, me.ID, req.Rating, req.Comment,
	).Scan(&rv.ID, &rv.ProductID, &rv.UserID, &rv.Rating, &rv.Comment, &rv.CreatedAt)
	if err != nil {
		serverError(w, err)
		return
	}
	rv.AuthorName = me.FullName
	writeJSON(w, http.StatusCreated, rv)
}

// DELETE /api/reviews/{id} — пікір авторы немесе әкімші жоя алады
func (s *Server) handleDeleteReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	me := currentUser(r)
	var authorID *int
	err := s.db.QueryRow(r.Context(), `SELECT user_id FROM reviews WHERE id = $1`, id).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "пікір табылмады")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if me.Role != "admin" && (authorID == nil || *authorID != me.ID) {
		writeError(w, http.StatusForbidden, "тек өз пікіріңізді жоя аласыз")
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM reviews WHERE id = $1`, id); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
