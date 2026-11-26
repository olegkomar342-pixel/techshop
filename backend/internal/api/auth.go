package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Phone     *string   `json:"phone"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type authResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// ---------------------------------------------------------------------
// JWT
// ---------------------------------------------------------------------

type claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func (s *Server) issueToken(u User) (string, error) {
	now := time.Now()
	c := claims{
		Role: u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprint(u.ID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.jwtTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.jwtSecret)
}

func (s *Server) parseToken(raw string) (userID int, err error) {
	c := &claims{}
	_, err = jwt.ParseWithClaims(raw, c, func(t *jwt.Token) (any, error) {
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return 0, err
	}
	_, err = fmt.Sscan(c.Subject, &userID)
	return userID, err
}

// ---------------------------------------------------------------------
// Middleware: requireAuth / requireAdmin
// ---------------------------------------------------------------------

type ctxKey struct{}

// currentUser middleware орнатқан пайдаланушыны қайтарады.
func currentUser(r *http.Request) User {
	return r.Context().Value(ctxKey{}).(User)
}

// requireAuth "Authorization: Bearer <token>" тақырыбын тексереді.
// Рөл токеннен емес, дерекқордан оқылады — рөл өзгерсе, бірден күшіне енеді.
func (s *Server) requireAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || raw == "" {
			writeError(w, http.StatusUnauthorized, "жүйеге кіру қажет")
			return
		}
		id, err := s.parseToken(raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "токен жарамсыз немесе мерзімі өткен")
			return
		}
		u, err := s.getUser(r.Context(), id)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "пайдаланушы табылмады")
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.Handler {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "admin" {
			writeError(w, http.StatusForbidden, "тек әкімшіге рұқсат етілген")
			return
		}
		next(w, r)
	})
}

func (s *Server) getUser(ctx context.Context, id int) (User, error) {
	var u User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, full_name, phone, role, created_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Role, &u.CreatedAt)
	return u, err
}

// ---------------------------------------------------------------------
// POST /api/auth/register
// ---------------------------------------------------------------------

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = normalizeEmail(req.Email)
	req.FullName = strings.TrimSpace(req.FullName)
	req.Phone = strings.TrimSpace(req.Phone)

	v := validator{}
	v.email("email", req.Email)
	v.password("password", req.Password)
	v.length("full_name", req.FullName, 2, 100)
	v.length("phone", req.Phone, 0, 30)
	if !v.ok(w) {
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		serverError(w, err)
		return
	}

	var u User
	err = s.db.QueryRow(r.Context(), `
		INSERT INTO users (email, password_hash, full_name, phone)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		ON CONFLICT (email) DO NOTHING
		RETURNING id, email, full_name, phone, role, created_at`,
		req.Email, string(hash), req.FullName, req.Phone,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "бұл email бойынша тіркелген пайдаланушы бар")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	s.respondWithToken(w, http.StatusCreated, u)
}

// ---------------------------------------------------------------------
// POST /api/auth/login
// ---------------------------------------------------------------------

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email мен құпиясөзді енгізіңіз")
		return
	}

	var u User
	var hash string
	err := s.db.QueryRow(r.Context(), `
		SELECT id, email, full_name, phone, role, created_at, password_hash
		FROM users WHERE email = $1`, req.Email,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Role, &u.CreatedAt, &hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		serverError(w, err)
		return
	}
	// Email жоқ па, әлде құпиясөз қате ме — бірдей хабар (пайдаланушыларды анықтауға жол бермейміз).
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "email немесе құпиясөз қате")
		return
	}
	s.respondWithToken(w, http.StatusOK, u)
}

func (s *Server) respondWithToken(w http.ResponseWriter, status int, u User) {
	token, err := s.issueToken(u)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, status, authResponse{Token: token, User: u})
}

// ---------------------------------------------------------------------
// GET/PUT /api/me — пайдаланушы кабинеті
// ---------------------------------------------------------------------

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

type updateMeRequest struct {
	FullName        string `json:"full_name"`
	Phone           string `json:"phone"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	me := currentUser(r)
	var req updateMeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.FullName = strings.TrimSpace(req.FullName)
	req.Phone = strings.TrimSpace(req.Phone)

	v := validator{}
	v.length("full_name", req.FullName, 2, 100)
	v.length("phone", req.Phone, 0, 30)
	if req.NewPassword != "" {
		v.password("new_password", req.NewPassword)
	}
	if !v.ok(w) {
		return
	}

	var newHash *string
	if req.NewPassword != "" {
		var cur string
		if err := s.db.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id = $1`, me.ID).Scan(&cur); err != nil {
			serverError(w, err)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(cur), []byte(req.CurrentPassword)) != nil {
			writeError(w, http.StatusBadRequest, "ағымдағы құпиясөз қате")
			return
		}
		h, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			serverError(w, err)
			return
		}
		hs := string(h)
		newHash = &hs
	}

	var u User
	err := s.db.QueryRow(r.Context(), `
		UPDATE users
		SET full_name = $2, phone = NULLIF($3, ''), password_hash = COALESCE($4, password_hash)
		WHERE id = $1
		RETURNING id, email, full_name, phone, role, created_at`,
		me.ID, req.FullName, req.Phone, newHash,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Role, &u.CreatedAt)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}
