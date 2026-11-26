package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Product struct {
	ID           int       `json:"id"`
	CategoryID   *int      `json:"category_id"`
	CategoryName *string   `json:"category_name"`
	CategorySlug *string   `json:"category_slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Price        float64   `json:"price"`
	Stock        int       `json:"stock"`
	ImageURL     *string   `json:"image_url"`
	IsActive     bool      `json:"is_active"`
	AvgRating    *float64  `json:"avg_rating"`
	ReviewsCount int       `json:"reviews_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Review struct {
	ID         int       `json:"id"`
	ProductID  int       `json:"product_id"`
	UserID     *int      `json:"user_id"`
	AuthorName string    `json:"author_name"`
	Rating     int       `json:"rating"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

type ProductDetails struct {
	Product
	Reviews []Review `json:"reviews"`
}

type ProductPage struct {
	Items []Product `json:"items"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
	Pages int       `json:"pages"`
}

// Барлық тауар сұраныстарына ортақ SELECT бөлігі (санат атауы + орташа рейтинг).
const productSelect = `
	SELECT p.id, p.category_id, c.name, c.slug, p.name, p.description, p.price, p.stock,
	       p.image_url, p.is_active, r.avg_rating, COALESCE(r.cnt, 0), p.created_at, p.updated_at
	FROM products p
	LEFT JOIN categories c ON c.id = p.category_id
	LEFT JOIN (
		SELECT product_id, ROUND(AVG(rating), 1)::float8 AS avg_rating, COUNT(*) AS cnt
		FROM reviews GROUP BY product_id
	) r ON r.product_id = p.id`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.CategoryID, &p.CategoryName, &p.CategorySlug, &p.Name, &p.Description, &p.Price,
		&p.Stock, &p.ImageURL, &p.IsActive, &p.AvgRating, &p.ReviewsCount, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// Сұрыптау тек осы тізімнен таңдалады (SQL-инъекциядан қорғау).
var productSorts = map[string]string{
	"newest":     "p.created_at DESC, p.id DESC",
	"price_asc":  "p.price ASC, p.id",
	"price_desc": "p.price DESC, p.id",
	"rating":     "r.avg_rating DESC NULLS LAST, r.cnt DESC NULLS LAST, p.id",
	"name":       "p.name ASC, p.id",
}

type productFilter struct {
	Query           string
	CategoryID      int
	MinPrice        *float64
	MaxPrice        *float64
	InStock         bool
	Sort            string
	Page, Limit     int
	IncludeInactive bool
}

// parseProductFilter URL параметрлерін оқиды және тексереді.
func parseProductFilter(q url.Values) (productFilter, error) {
	f := productFilter{
		Query: strings.TrimSpace(q.Get("q")),
		Sort:  q.Get("sort"),
		Page:  1,
		Limit: 12,
	}
	if f.Sort == "" {
		f.Sort = "newest"
	}
	if _, ok := productSorts[f.Sort]; !ok {
		return f, fmt.Errorf("sort параметрі қате (newest, price_asc, price_desc, rating, name)")
	}
	intParam := func(name string, dst *int, min, max int) error {
		if s := q.Get(name); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < min || n > max {
				return fmt.Errorf("%s параметрі қате", name)
			}
			*dst = n
		}
		return nil
	}
	floatParam := func(name string) (*float64, error) {
		s := q.Get(name)
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("%s параметрі қате", name)
		}
		return &v, nil
	}
	var err error
	if err = intParam("category", &f.CategoryID, 1, math.MaxInt32); err != nil {
		return f, err
	}
	if err = intParam("page", &f.Page, 1, 100000); err != nil {
		return f, err
	}
	if err = intParam("limit", &f.Limit, 1, 100); err != nil {
		return f, err
	}
	if f.MinPrice, err = floatParam("min_price"); err != nil {
		return f, err
	}
	if f.MaxPrice, err = floatParam("max_price"); err != nil {
		return f, err
	}
	f.InStock = q.Get("in_stock") == "1" || q.Get("in_stock") == "true"
	return f, nil
}

// buildTSQuery пайдаланушы енгізген мәтінді to_tsquery үшін қауіпсіз жолға айналдырады:
// "samsung гал" → "samsung:* & гал:*" (сөз басымен іздеу).
func buildTSQuery(input string) string {
	words := strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) > 8 {
		words = words[:8]
	}
	for i, w := range words {
		words[i] = w + ":*"
	}
	return strings.Join(words, " & ")
}

func (s *Server) listProducts(ctx context.Context, f productFilter) (ProductPage, error) {
	var where []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	if !f.IncludeInactive {
		where = append(where, "p.is_active")
	}
	if f.Query != "" {
		if ts := buildTSQuery(f.Query); ts != "" {
			// Толық мәтінді іздеу (GIN индексі) + атауы бойынша ішінара сәйкестік.
			where = append(where, fmt.Sprintf(
				"(to_tsvector('simple', p.name || ' ' || p.description) @@ to_tsquery('simple', %s) OR p.name ILIKE %s)",
				arg(ts), arg("%"+escapeLike(f.Query)+"%")))
		}
	}
	if f.CategoryID > 0 {
		// Санат және оның барлық ішкі санаттары
		where = append(where, fmt.Sprintf(`p.category_id IN (
			WITH RECURSIVE tree AS (
				SELECT id FROM categories WHERE id = %s
				UNION
				SELECT c.id FROM categories c JOIN tree t ON c.parent_id = t.id
			) SELECT id FROM tree)`, arg(f.CategoryID)))
	}
	if f.MinPrice != nil {
		where = append(where, "p.price >= "+arg(*f.MinPrice))
	}
	if f.MaxPrice != nil {
		where = append(where, "p.price <= "+arg(*f.MaxPrice))
	}
	if f.InStock {
		where = append(where, "p.stock > 0")
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	page := ProductPage{Items: []Product{}, Page: f.Page, Limit: f.Limit}
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM products p"+whereSQL, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	page.Pages = (page.Total + f.Limit - 1) / f.Limit

	sql := productSelect + whereSQL + " ORDER BY " + productSorts[f.Sort] +
		" LIMIT " + arg(f.Limit) + " OFFSET " + arg((f.Page-1)*f.Limit)
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, p)
	}
	return page, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// GET /api/products?q=&category=&min_price=&max_price=&in_stock=1&sort=&page=&limit=
func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	f, err := parseProductFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.listProducts(r.Context(), f)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// GET /api/products/{id} — тауар және оның пікірлері
func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, err := scanProduct(s.db.QueryRow(r.Context(), productSelect+" WHERE p.id = $1 AND p.is_active", id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "тауар табылмады")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	reviews, err := s.productReviews(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ProductDetails{Product: p, Reviews: reviews})
}

func (s *Server) productReviews(ctx context.Context, productID int) ([]Review, error) {
	rows, err := s.db.Query(ctx, `
		SELECT rv.id, rv.product_id, rv.user_id, COALESCE(u.full_name, 'Жойылған пайдаланушы'),
		       rv.rating, rv.comment, rv.created_at
		FROM reviews rv LEFT JOIN users u ON u.id = rv.user_id
		WHERE rv.product_id = $1
		ORDER BY rv.created_at DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Review{}
	for rows.Next() {
		var rv Review
		if err := rows.Scan(&rv.ID, &rv.ProductID, &rv.UserID, &rv.AuthorName, &rv.Rating, &rv.Comment, &rv.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, rv)
	}
	return list, rows.Err()
}

// ---------------------------------------------------------------------
// Санаттар
// ---------------------------------------------------------------------

type Category struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	ParentID     *int   `json:"parent_id"`
	ProductCount int    `json:"product_count"`
}

// GET /api/categories
func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT c.id, c.name, c.slug, c.parent_id,
		       (SELECT COUNT(*) FROM products p WHERE p.category_id = c.id AND p.is_active)
		FROM categories c
		ORDER BY COALESCE(c.parent_id, c.id), c.parent_id NULLS FIRST, c.name`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	list := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.ParentID, &c.ProductCount); err != nil {
			serverError(w, err)
			return
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// pgErrCode PostgreSQL қатесінің кодын қайтарады (мыс. 23505 — unique, 23503 — foreign key).
func pgErrCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
