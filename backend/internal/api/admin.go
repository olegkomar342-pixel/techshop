package api

import (
	"math"
	"net/http"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------
// Статистика
// ---------------------------------------------------------------------

type adminStats struct {
	Products       int            `json:"products"`
	ActiveProducts int            `json:"active_products"`
	LowStock       int            `json:"low_stock"`
	Users          int            `json:"users"`
	Revenue        float64        `json:"revenue"`
	OrdersByStatus map[string]int `json:"orders_by_status"`
}

// GET /api/admin/stats
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	st := adminStats{OrdersByStatus: map[string]int{}}
	for _, status := range orderStatuses {
		st.OrdersByStatus[status] = 0
	}
	err := s.db.QueryRow(r.Context(), `
		SELECT (SELECT COUNT(*) FROM products),
		       (SELECT COUNT(*) FROM products WHERE is_active),
		       (SELECT COUNT(*) FROM products WHERE is_active AND stock <= 3),
		       (SELECT COUNT(*) FROM users),
		       (SELECT COALESCE(SUM(total), 0) FROM orders WHERE status IN ('paid', 'shipped', 'delivered'))`,
	).Scan(&st.Products, &st.ActiveProducts, &st.LowStock, &st.Users, &st.Revenue)
	if err != nil {
		serverError(w, err)
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT status, COUNT(*) FROM orders GROUP BY status`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			serverError(w, err)
			return
		}
		st.OrdersByStatus[status] = n
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ---------------------------------------------------------------------
// Тауарлар
// ---------------------------------------------------------------------

// GET /api/admin/products — жасырылғандарын қоса барлық тауарлар
func (s *Server) handleAdminListProducts(w http.ResponseWriter, r *http.Request) {
	f, err := parseProductFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.Query().Get("limit") == "" {
		f.Limit = 100
	}
	if r.URL.Query().Get("sort") == "" {
		f.Sort = "name"
	}
	f.IncludeInactive = true
	page, err := s.listProducts(r.Context(), f)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type productInput struct {
	CategoryID  *int    `json:"category_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	ImageURL    string  `json:"image_url"`
	IsActive    *bool   `json:"is_active"`
}

func (in *productInput) validate(w http.ResponseWriter) bool {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.ImageURL = strings.TrimSpace(in.ImageURL)
	in.Price = math.Round(in.Price*100) / 100
	if in.IsActive == nil {
		t := true
		in.IsActive = &t
	}
	v := validator{}
	v.length("name", in.Name, 2, 200)
	v.length("description", in.Description, 0, 5000)
	v.check(in.Price >= 0 && in.Price < 1e10, "price", "баға 0-ден кем болмауы керек")
	v.check(in.Stock >= 0 && in.Stock <= 1_000_000, "stock", "қалдық 0-ден кем болмауы керек")
	v.check(in.CategoryID == nil || *in.CategoryID > 0, "category_id", "санат қате")
	v.check(in.ImageURL == "" || strings.HasPrefix(in.ImageURL, "https://") ||
		strings.HasPrefix(in.ImageURL, "http://") || strings.HasPrefix(in.ImageURL, "/"),
		"image_url", "сурет сілтемесі http(s):// немесе / белгісінен басталуы керек")
	v.length("image_url", in.ImageURL, 0, 1000)
	return v.ok(w)
}

// POST /api/admin/products
func (s *Server) handleAdminCreateProduct(w http.ResponseWriter, r *http.Request) {
	var in productInput
	if !decodeJSON(w, r, &in) || !in.validate(w) {
		return
	}
	var id int
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO products (category_id, name, description, price, stock, image_url, is_active)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7) RETURNING id`,
		in.CategoryID, in.Name, in.Description, in.Price, in.Stock, in.ImageURL, *in.IsActive,
	).Scan(&id)
	if s.productWriteError(w, err) {
		return
	}
	s.respondAdminProduct(w, r, http.StatusCreated, id)
}

// PUT /api/admin/products/{id} — өңдеу; is_active=false тауарды каталогтан жасырады
func (s *Server) handleAdminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in productInput
	if !decodeJSON(w, r, &in) || !in.validate(w) {
		return
	}
	tag, err := s.db.Exec(r.Context(), `
		UPDATE products SET category_id = $2, name = $3, description = $4, price = $5,
		       stock = $6, image_url = NULLIF($7, ''), is_active = $8
		WHERE id = $1`,
		id, in.CategoryID, in.Name, in.Description, in.Price, in.Stock, in.ImageURL, *in.IsActive)
	if s.productWriteError(w, err) {
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "тауар табылмады")
		return
	}
	s.respondAdminProduct(w, r, http.StatusOK, id)
}

func (s *Server) productWriteError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if pgErrCode(err) == "23503" { // foreign_key_violation
		writeError(w, http.StatusBadRequest, "мұндай санат жоқ")
	} else {
		serverError(w, err)
	}
	return true
}

func (s *Server) respondAdminProduct(w http.ResponseWriter, r *http.Request, status, id int) {
	p, err := scanProduct(s.db.QueryRow(r.Context(), productSelect+" WHERE p.id = $1", id))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, status, p)
}

// ---------------------------------------------------------------------
// Санаттар
// ---------------------------------------------------------------------

type categoryInput struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	ParentID *int   `json:"parent_id"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// POST /api/admin/categories
func (s *Server) handleAdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	var in categoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	v := validator{}
	v.length("name", in.Name, 2, 100)
	v.length("slug", in.Slug, 2, 100)
	v.check(in.Slug == "" || slugRe.MatchString(in.Slug), "slug", "тек латын әріптері, сандар және дефис")
	if !v.ok(w) {
		return
	}
	var c Category
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO categories (name, slug, parent_id) VALUES ($1, $2, $3)
		RETURNING id, name, slug, parent_id`, in.Name, in.Slug, in.ParentID,
	).Scan(&c.ID, &c.Name, &c.Slug, &c.ParentID)
	switch pgErrCode(err) {
	case "":
	case "23505":
		writeError(w, http.StatusConflict, "мұндай slug бар")
		return
	case "23503":
		writeError(w, http.StatusBadRequest, "ата-санат табылмады")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// DELETE /api/admin/categories/{id} — тауарлар мен ішкі санаттардың category/parent мәні NULL болады
func (s *Server) handleAdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM categories WHERE id = $1`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "санат табылмады")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------
// Тапсырыстар
// ---------------------------------------------------------------------

// GET /api/admin/orders?status=new
func (s *Server) handleAdminListOrders(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	var (
		orders []Order
		err    error
	)
	if status == "" {
		orders, err = s.queryOrders(r.Context(), "")
	} else {
		v := validator{}
		v.oneOf("status", status, orderStatuses...)
		if !v.ok(w) {
			return
		}
		orders, err = s.queryOrders(r.Context(), "WHERE o.status = $1", status)
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// GET /api/admin/orders/{id}
func (s *Server) handleAdminGetOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	orders, err := s.queryOrders(r.Context(), "WHERE o.id = $1", id)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(orders) == 0 {
		writeError(w, http.StatusNotFound, "тапсырыс табылмады")
		return
	}
	writeJSON(w, http.StatusOK, orders[0])
}

// PUT /api/admin/orders/{id}/status { status }
// Онлайн төлем жоқ, сондықтан «paid» күйін әкімші қолмен қояды.
func (s *Server) handleAdminSetOrderStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	v := validator{}
	v.oneOf("status", req.Status, orderStatuses...)
	if !v.ok(w) {
		return
	}
	err := s.changeOrderStatus(r.Context(), id, req.Status, func(orderLock) error { return nil })
	s.respondStatusChange(w, r, id, err)
}
