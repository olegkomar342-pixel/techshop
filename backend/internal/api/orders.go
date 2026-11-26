package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Жеткізу шарттары
const (
	CourierCost      = 1500  // курьерлік жеткізу бағасы, ₸
	FreeDeliveryFrom = 50000 // осы сомадан бастап жеткізу тегін, ₸
	PickupAddress    = "Өзі алып кету: Алматы қ., Сәтбаев к. 22, «TechShop» дүкені"
)

var orderStatuses = []string{"new", "paid", "shipped", "delivered", "cancelled"}

type OrderItem struct {
	ID          int     `json:"id"`
	ProductID   *int    `json:"product_id"`
	ProductName string  `json:"product_name"`
	UnitPrice   float64 `json:"unit_price"`
	Quantity    int     `json:"quantity"`
	Subtotal    float64 `json:"subtotal"`
}

type Order struct {
	ID              int         `json:"id"`
	UserID          *int        `json:"user_id"`
	UserEmail       *string     `json:"user_email,omitempty"`
	Status          string      `json:"status"`
	FullName        string      `json:"full_name"`
	Phone           string      `json:"phone"`
	ShippingAddress string      `json:"shipping_address"`
	DeliveryMethod  string      `json:"delivery_method"`
	PaymentMethod   string      `json:"payment_method"`
	DeliveryCost    float64     `json:"delivery_cost"`
	Total           float64     `json:"total"`
	Comment         string      `json:"comment"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	Items           []OrderItem `json:"items"`
}

const orderSelect = `
	SELECT o.id, o.user_id, u.email, o.status, o.full_name, o.phone, o.shipping_address,
	       o.delivery_method, o.payment_method, o.delivery_cost, o.total, o.comment,
	       o.created_at, o.updated_at
	FROM orders o LEFT JOIN users u ON u.id = o.user_id`

// queryOrders тапсырыстарды позицияларымен бірге жүктейді.
func (s *Server) queryOrders(ctx context.Context, where string, args ...any) ([]Order, error) {
	rows, err := s.db.Query(ctx, orderSelect+" "+where+" ORDER BY o.created_at DESC, o.id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []Order{}
	index := map[int]int{}
	var ids []int
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.UserEmail, &o.Status, &o.FullName, &o.Phone,
			&o.ShippingAddress, &o.DeliveryMethod, &o.PaymentMethod, &o.DeliveryCost, &o.Total,
			&o.Comment, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Items = []OrderItem{}
		index[o.ID] = len(orders)
		ids = append(ids, o.ID)
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil || len(ids) == 0 {
		return orders, err
	}

	// Барлық тапсырыстың позицияларын бір сұраныспен аламыз
	itemRows, err := s.db.Query(ctx, `
		SELECT order_id, id, product_id, product_name, unit_price, quantity, unit_price * quantity
		FROM order_items WHERE order_id = ANY($1) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var orderID int
		var it OrderItem
		if err := itemRows.Scan(&orderID, &it.ID, &it.ProductID, &it.ProductName, &it.UnitPrice,
			&it.Quantity, &it.Subtotal); err != nil {
			return nil, err
		}
		o := &orders[index[orderID]]
		o.Items = append(o.Items, it)
	}
	return orders, itemRows.Err()
}

// ---------------------------------------------------------------------
// POST /api/orders — тапсырысты рәсімдеу (бір транзакцияда)
// ---------------------------------------------------------------------

type createOrderRequest struct {
	FullName        string `json:"full_name"`
	Phone           string `json:"phone"`
	ShippingAddress string `json:"shipping_address"`
	DeliveryMethod  string `json:"delivery_method"`
	PaymentMethod   string `json:"payment_method"`
	Comment         string `json:"comment"`
}

// stockError тапсырыс кезінде қалдық жетпесе қайтарылады (транзакция кері қайтарылады).
type stockError struct{ msg string }

func (e stockError) Error() string { return e.msg }

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	me := currentUser(r)
	var req createOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.FullName = strings.TrimSpace(req.FullName)
	req.Phone = strings.TrimSpace(req.Phone)
	req.ShippingAddress = strings.TrimSpace(req.ShippingAddress)
	req.Comment = strings.TrimSpace(req.Comment)
	if req.FullName == "" {
		req.FullName = me.FullName
	}
	if req.Phone == "" && me.Phone != nil {
		req.Phone = *me.Phone
	}
	if req.DeliveryMethod == "" {
		req.DeliveryMethod = "courier"
	}
	if req.PaymentMethod == "" {
		req.PaymentMethod = "cash"
	}
	if req.DeliveryMethod == "pickup" {
		req.ShippingAddress = PickupAddress
	}

	v := validator{}
	v.length("full_name", req.FullName, 2, 100)
	v.length("phone", req.Phone, 5, 30)
	v.length("shipping_address", req.ShippingAddress, 5, 300)
	v.oneOf("delivery_method", req.DeliveryMethod, "courier", "pickup")
	v.oneOf("payment_method", req.PaymentMethod, "cash", "card")
	v.length("comment", req.Comment, 0, 500)
	if !v.ok(w) {
		return
	}

	orderID, err := s.createOrder(r.Context(), me.ID, req)
	var se stockError
	switch {
	case errors.As(err, &se):
		writeError(w, http.StatusBadRequest, se.msg)
		return
	case err != nil:
		serverError(w, err)
		return
	}

	orders, err := s.queryOrders(r.Context(), "WHERE o.id = $1", orderID)
	if err != nil || len(orders) == 0 {
		serverError(w, fmt.Errorf("жаңа тапсырысты оқу: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, orders[0])
}

// createOrder: қалдықты тексеру → тапсырыс жасау → қалдықты азайту → себетті тазалау.
// Кез келген қадамда қате болса — бәрі кері қайтарылады (ROLLBACK).
func (s *Server) createOrder(ctx context.Context, userID int, req createOrderRequest) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) // Commit сәтті болса, Rollback ештеңе істемейді

	// 1. Себеттегі тауарларды оқып, жолдарды бұғаттаймыз (FOR UPDATE).
	//    Екі сатып алушы соңғы тауарды бір мезгілде алғысы келсе, екіншісі
	//    біріншінің транзакциясы аяқталғанша күтеді және жаңартылған қалдықты көреді.
	//    ORDER BY p.id — бұғаттау реті бірдей болса, deadlock болмайды.
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.name, p.stock, p.is_active, ci.quantity
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		WHERE ci.user_id = $1
		ORDER BY p.id
		FOR UPDATE`, userID)
	if err != nil {
		return 0, err
	}
	type line struct {
		id, stock, qty int
		name           string
		active         bool
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.id, &l.name, &l.stock, &l.active, &l.qty); err != nil {
			rows.Close()
			return 0, err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(lines) == 0 {
		return 0, stockError{"себет бос"}
	}
	for _, l := range lines {
		if !l.active {
			return 0, stockError{fmt.Sprintf("«%s» тауары сатылымнан алынды — оны себеттен жойыңыз", l.name)}
		}
		if l.qty > l.stock {
			return 0, stockError{fmt.Sprintf("«%s» тауары қоймада жеткіліксіз (қалғаны: %d)", l.name, l.stock)}
		}
	}

	// 2. Тапсырыс жасаймыз (сома төменде есептеледі)
	var orderID int
	err = tx.QueryRow(ctx, `
		INSERT INTO orders (user_id, full_name, phone, shipping_address, delivery_method, payment_method, comment, total)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0) RETURNING id`,
		userID, req.FullName, req.Phone, req.ShippingAddress, req.DeliveryMethod, req.PaymentMethod, req.Comment,
	).Scan(&orderID)
	if err != nil {
		return 0, err
	}

	// 3. Позициялар: атауы мен бағасы сатып алу сәтіндегі күйінде көшіріледі
	if _, err = tx.Exec(ctx, `
		INSERT INTO order_items (order_id, product_id, product_name, unit_price, quantity)
		SELECT $1, p.id, p.name, p.price, ci.quantity
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		WHERE ci.user_id = $2
		ORDER BY p.id`, orderID, userID); err != nil {
		return 0, err
	}

	// 4. Қойма қалдығын азайту (CHECK stock >= 0 қосымша қорғаныс береді)
	if _, err = tx.Exec(ctx, `
		UPDATE products p SET stock = p.stock - ci.quantity
		FROM cart_items ci
		WHERE ci.product_id = p.id AND ci.user_id = $1`, userID); err != nil {
		return 0, err
	}

	// 5. Жеткізу құны мен жалпы соманы NUMERIC арқылы есептеу
	if _, err = tx.Exec(ctx, `
		WITH s AS (SELECT SUM(unit_price * quantity) AS sub FROM order_items WHERE order_id = $1)
		UPDATE orders o SET
			delivery_cost = CASE WHEN o.delivery_method = 'courier' AND s.sub < $2 THEN $3 ELSE 0 END,
			total = s.sub + CASE WHEN o.delivery_method = 'courier' AND s.sub < $2 THEN $3 ELSE 0 END
		FROM s WHERE o.id = $1`, orderID, FreeDeliveryFrom, CourierCost); err != nil {
		return 0, err
	}

	// 6. Себетті тазалау
	if _, err = tx.Exec(ctx, `DELETE FROM cart_items WHERE user_id = $1`, userID); err != nil {
		return 0, err
	}

	return orderID, tx.Commit(ctx)
}

// GET /api/orders — пайдаланушының тапсырыстар тарихы
func (s *Server) handleListMyOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := s.queryOrders(r.Context(), "WHERE o.user_id = $1", currentUser(r).ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// GET /api/orders/{id}
func (s *Server) handleGetMyOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	orders, err := s.queryOrders(r.Context(), "WHERE o.id = $1 AND o.user_id = $2", id, currentUser(r).ID)
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

// POST /api/orders/{id}/cancel — тек «new» күйіндегі тапсырысты сатып алушы өзі болдырмай алады
func (s *Server) handleCancelMyOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	userID := currentUser(r).ID
	err := s.changeOrderStatus(r.Context(), id, "cancelled", func(o orderLock) error {
		if o.userID == nil || *o.userID != userID {
			return errNotFound
		}
		if o.status != "new" {
			return statusError{"тек «жаңа» күйіндегі тапсырысты болдырмауға болады"}
		}
		return nil
	})
	s.respondStatusChange(w, r, id, err)
}

// ---------------------------------------------------------------------
// Күйді өзгерту (сатып алушы мен әкімшіге ортақ)
// ---------------------------------------------------------------------

var errNotFound = errors.New("not found")

type statusError struct{ msg string }

func (e statusError) Error() string { return e.msg }

type orderLock struct {
	userID *int
	status string
}

// changeOrderStatus тапсырыс жолын бұғаттап, күйін ауыстырады.
// «cancelled» күйіне өткенде тауарлар қоймаға қайтарылады.
func (s *Server) changeOrderStatus(ctx context.Context, orderID int, newStatus string, check func(orderLock) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var o orderLock
	err = tx.QueryRow(ctx, `SELECT user_id, status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&o.userID, &o.status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if err := check(o); err != nil {
		return err
	}
	if o.status == newStatus {
		return nil
	}
	if o.status == "cancelled" {
		return statusError{"болдырылмаған тапсырыстың күйін өзгертуге болмайды"}
	}
	if newStatus == "cancelled" {
		if _, err := tx.Exec(ctx, `
			UPDATE products p SET stock = p.stock + oi.quantity
			FROM order_items oi
			WHERE oi.order_id = $1 AND oi.product_id = p.id`, orderID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET status = $2 WHERE id = $1`, orderID, newStatus); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) respondStatusChange(w http.ResponseWriter, r *http.Request, orderID int, err error) {
	var se statusError
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "тапсырыс табылмады")
		return
	case errors.As(err, &se):
		writeError(w, http.StatusBadRequest, se.msg)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	orders, err := s.queryOrders(r.Context(), "WHERE o.id = $1", orderID)
	if err != nil || len(orders) == 0 {
		serverError(w, fmt.Errorf("тапсырысты оқу: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, orders[0])
}
