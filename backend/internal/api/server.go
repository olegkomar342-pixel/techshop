// Package api — интернет-дүкеннің REST API-ы (JSON, HTTP).
package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server барлық өңдеушілерге (handler) ортақ тәуелділіктерді сақтайды.
type Server struct {
	db        *pgxpool.Pool
	jwtSecret []byte
	jwtTTL    time.Duration
}

func NewServer(db *pgxpool.Pool, jwtSecret string, jwtTTL time.Duration) *Server {
	return &Server{db: db, jwtSecret: []byte(jwtSecret), jwtTTL: jwtTTL}
}

// Routes барлық эндпоинттерді тіркейді. Go 1.22+ ServeMux әдіс пен {id} параметрлерін қолдайды.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Ашық эндпоинттер
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/categories", s.handleListCategories)
	mux.HandleFunc("GET /api/products", s.handleListProducts)
	mux.HandleFunc("GET /api/products/{id}", s.handleGetProduct)
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)

	// Кіруді талап ететін эндпоинттер
	mux.Handle("GET /api/me", s.requireAuth(s.handleGetMe))
	mux.Handle("PUT /api/me", s.requireAuth(s.handleUpdateMe))

	mux.Handle("GET /api/cart", s.requireAuth(s.handleGetCart))
	mux.Handle("POST /api/cart", s.requireAuth(s.handleAddToCart))
	mux.Handle("PUT /api/cart/{productId}", s.requireAuth(s.handleUpdateCartItem))
	mux.Handle("DELETE /api/cart/{productId}", s.requireAuth(s.handleDeleteCartItem))
	mux.Handle("DELETE /api/cart", s.requireAuth(s.handleClearCart))

	mux.Handle("POST /api/orders", s.requireAuth(s.handleCreateOrder))
	mux.Handle("GET /api/orders", s.requireAuth(s.handleListMyOrders))
	mux.Handle("GET /api/orders/{id}", s.requireAuth(s.handleGetMyOrder))
	mux.Handle("POST /api/orders/{id}/cancel", s.requireAuth(s.handleCancelMyOrder))

	mux.Handle("POST /api/products/{id}/reviews", s.requireAuth(s.handleUpsertReview))
	mux.Handle("DELETE /api/reviews/{id}", s.requireAuth(s.handleDeleteReview))

	// Әкімші панелі
	mux.Handle("GET /api/admin/stats", s.requireAdmin(s.handleAdminStats))
	mux.Handle("GET /api/admin/products", s.requireAdmin(s.handleAdminListProducts))
	mux.Handle("POST /api/admin/products", s.requireAdmin(s.handleAdminCreateProduct))
	mux.Handle("PUT /api/admin/products/{id}", s.requireAdmin(s.handleAdminUpdateProduct))
	mux.Handle("POST /api/admin/categories", s.requireAdmin(s.handleAdminCreateCategory))
	mux.Handle("DELETE /api/admin/categories/{id}", s.requireAdmin(s.handleAdminDeleteCategory))
	mux.Handle("GET /api/admin/orders", s.requireAdmin(s.handleAdminListOrders))
	mux.Handle("GET /api/admin/orders/{id}", s.requireAdmin(s.handleAdminGetOrder))
	mux.Handle("PUT /api/admin/orders/{id}/status", s.requireAdmin(s.handleAdminSetOrderStatus))

	// Белгісіз /api/... жолдары үшін JSON форматындағы 404
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "эндпоинт табылмады")
	})

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "дерекқорға қосылу жоқ")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
