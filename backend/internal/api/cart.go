package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

type CartItem struct {
	ProductID int     `json:"product_id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Quantity  int     `json:"quantity"`
	Stock     int     `json:"stock"`
	ImageURL  *string `json:"image_url"`
	Category  *string `json:"category_slug"`
	IsActive  bool    `json:"is_active"`
	Subtotal  float64 `json:"subtotal"`
}

type Cart struct {
	Items      []CartItem `json:"items"`
	TotalItems int        `json:"total_items"`
	Total      float64    `json:"total"`
}

const maxQuantity = 99

func (s *Server) loadCart(ctx context.Context, userID int) (Cart, error) {
	cart := Cart{Items: []CartItem{}}
	rows, err := s.db.Query(ctx, `
		SELECT p.id, p.name, p.price, ci.quantity, p.stock, p.image_url, c.slug, p.is_active,
		       p.price * ci.quantity
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE ci.user_id = $1
		ORDER BY ci.added_at, p.id`, userID)
	if err != nil {
		return cart, err
	}
	defer rows.Close()
	for rows.Next() {
		var it CartItem
		if err := rows.Scan(&it.ProductID, &it.Name, &it.Price, &it.Quantity, &it.Stock,
			&it.ImageURL, &it.Category, &it.IsActive, &it.Subtotal); err != nil {
			return cart, err
		}
		cart.Items = append(cart.Items, it)
	}
	if err := rows.Err(); err != nil {
		return cart, err
	}
	// Жалпы соманы NUMERIC дәлдігімен дерекқорда есептейміз.
	err = s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(ci.quantity), 0), COALESCE(SUM(p.price * ci.quantity), 0)
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		WHERE ci.user_id = $1`, userID).Scan(&cart.TotalItems, &cart.Total)
	return cart, err
}

func (s *Server) respondCart(w http.ResponseWriter, r *http.Request, status int) {
	cart, err := s.loadCart(r.Context(), currentUser(r).ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, status, cart)
}

// GET /api/cart
func (s *Server) handleGetCart(w http.ResponseWriter, r *http.Request) {
	s.respondCart(w, r, http.StatusOK)
}

type cartRequest struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

// POST /api/cart { product_id, quantity } — тауарды қосады (бар болса, санын арттырады)
func (s *Server) handleAddToCart(w http.ResponseWriter, r *http.Request) {
	var req cartRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Quantity == 0 {
		req.Quantity = 1
	}
	v := validator{}
	v.check(req.ProductID > 0, "product_id", "міндетті өріс")
	v.check(req.Quantity >= 1 && req.Quantity <= maxQuantity, "quantity", "1-ден 99-ға дейін")
	if !v.ok(w) {
		return
	}
	userID := currentUser(r).ID

	stock, ok := s.activeProductStock(w, r, req.ProductID)
	if !ok {
		return
	}
	var current int
	err := s.db.QueryRow(r.Context(),
		`SELECT quantity FROM cart_items WHERE user_id = $1 AND product_id = $2`,
		userID, req.ProductID).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		serverError(w, err)
		return
	}
	if current+req.Quantity > stock {
		writeError(w, http.StatusBadRequest, "қоймада жеткілікті тауар жоқ")
		return
	}
	_, err = s.db.Exec(r.Context(), `
		INSERT INTO cart_items (user_id, product_id, quantity) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, product_id)
		DO UPDATE SET quantity = LEAST(cart_items.quantity + EXCLUDED.quantity, $4)`,
		userID, req.ProductID, req.Quantity, maxQuantity)
	if err != nil {
		serverError(w, err)
		return
	}
	s.respondCart(w, r, http.StatusCreated)
}

// PUT /api/cart/{productId} { quantity } — санын өзгерту
func (s *Server) handleUpdateCartItem(w http.ResponseWriter, r *http.Request) {
	productID, ok := pathID(w, r, "productId")
	if !ok {
		return
	}
	var req cartRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	v := validator{}
	v.check(req.Quantity >= 1 && req.Quantity <= maxQuantity, "quantity", "1-ден 99-ға дейін")
	if !v.ok(w) {
		return
	}
	stock, ok := s.activeProductStock(w, r, productID)
	if !ok {
		return
	}
	if req.Quantity > stock {
		writeError(w, http.StatusBadRequest, "қоймада жеткілікті тауар жоқ")
		return
	}
	tag, err := s.db.Exec(r.Context(),
		`UPDATE cart_items SET quantity = $3 WHERE user_id = $1 AND product_id = $2`,
		currentUser(r).ID, productID, req.Quantity)
	if err != nil {
		serverError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "тауар себетте жоқ")
		return
	}
	s.respondCart(w, r, http.StatusOK)
}

// DELETE /api/cart/{productId}
func (s *Server) handleDeleteCartItem(w http.ResponseWriter, r *http.Request) {
	productID, ok := pathID(w, r, "productId")
	if !ok {
		return
	}
	tag, err := s.db.Exec(r.Context(),
		`DELETE FROM cart_items WHERE user_id = $1 AND product_id = $2`, currentUser(r).ID, productID)
	if err != nil {
		serverError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "тауар себетте жоқ")
		return
	}
	s.respondCart(w, r, http.StatusOK)
}

// DELETE /api/cart — себетті толық тазалау
func (s *Server) handleClearCart(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.Exec(r.Context(), `DELETE FROM cart_items WHERE user_id = $1`, currentUser(r).ID); err != nil {
		serverError(w, err)
		return
	}
	s.respondCart(w, r, http.StatusOK)
}

// activeProductStock тауар бар және белсенді екенін тексеріп, қалдығын қайтарады.
func (s *Server) activeProductStock(w http.ResponseWriter, r *http.Request, productID int) (int, bool) {
	var stock int
	err := s.db.QueryRow(r.Context(),
		`SELECT stock FROM products WHERE id = $1 AND is_active`, productID).Scan(&stock)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "тауар табылмады")
		return 0, false
	}
	if err != nil {
		serverError(w, err)
		return 0, false
	}
	return stock, true
}
