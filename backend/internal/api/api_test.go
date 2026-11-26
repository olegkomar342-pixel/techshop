package api

// API автотесттері.
//
// Бірлік тесттері (дерекқорсыз):   go test ./...
// Толық тесттер (бөлек сынақ дерекқоры керек — ол ТАЗАЛАНАДЫ!):
//   TEST_DATABASE_URL="postgres://postgres:ПАРОЛЬ@localhost:5432/shop_test?sslmode=disable" go test ./... -v

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------
// Бірлік тесттері
// ---------------------------------------------------------------------

func TestBuildTSQuery(t *testing.T) {
	cases := map[string]string{
		"samsung":           "samsung:*",
		"  Apple  iPhone ":  "apple:* & iphone:*",
		"құлаққап":          "құлаққап:*",
		"a'; DROP TABLE --": "a:* & drop:* & table:*",
		"!!! &&& |||":       "",
	}
	for in, want := range cases {
		if got := buildTSQuery(in); got != want {
			t.Errorf("buildTSQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidator(t *testing.T) {
	v := validator{}
	v.email("email", "user@mail.kz")
	v.password("password", "secret1")
	v.length("full_name", "Аружан", 2, 100)
	if !v.valid() {
		t.Fatalf("дұрыс деректер қате деп табылды: %v", v.errs)
	}

	v = validator{}
	v.email("email", "not-an-email")
	v.password("password", "123")
	v.length("full_name", "", 2, 100)
	v.oneOf("status", "lost", orderStatuses...)
	for _, f := range []string{"email", "password", "full_name", "status"} {
		if _, ok := v.errs[f]; !ok {
			t.Errorf("%s өрісі үшін қате күтілді", f)
		}
	}
}

func TestParseProductFilter(t *testing.T) {
	if _, err := parseProductFilter(map[string][]string{"sort": {"price; DROP"}}); err == nil {
		t.Error("рұқсатсыз sort мәні қабылданбауы керек")
	}
	if _, err := parseProductFilter(map[string][]string{"min_price": {"-5"}}); err == nil {
		t.Error("теріс баға қабылданбауы керек")
	}
	f, err := parseProductFilter(map[string][]string{"page": {"2"}, "limit": {"5"}, "category": {"3"}})
	if err != nil || f.Page != 2 || f.Limit != 5 || f.CategoryID != 3 {
		t.Errorf("күтпеген нәтиже: %+v, %v", f, err)
	}
}

// ---------------------------------------------------------------------
// Интеграциялық тесттер (нақты PostgreSQL)
// ---------------------------------------------------------------------

type testEnv struct {
	t   *testing.T
	srv *httptest.Server
	db  *pgxpool.Pool
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL берілмеген — интеграциялық тест өткізілді")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("../../../database/01_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Схеманы қайта құрып, сынақ деректерін жүктейміз
	if _, err := db.Exec(ctx, "SET client_min_messages = warning;\n"+string(schema)); err != nil {
		t.Fatalf("схема: %v", err)
	}
	s := NewServer(db, "test-secret-test-secret", time.Hour)
	env := &testEnv{t: t, srv: httptest.NewServer(Recoverer(s.Routes())), db: db}
	t.Cleanup(func() {
		env.srv.Close()
		db.Close()
	})
	return env
}

// do сұраныс жіберіп, жауапты out-қа оқиды және статус кодын қайтарады.
func (e *testEnv) do(method, path, token string, body any, out any) int {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil && res.StatusCode != http.StatusNoContent {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func (e *testEnv) login(email, password string) string {
	e.t.Helper()
	var res authResponse
	if code := e.do("POST", "/api/auth/login", "", map[string]string{"email": email, "password": password}, &res); code != 200 {
		e.t.Fatalf("login %s: %d", email, code)
	}
	return res.Token
}

func (e *testEnv) register(email string) string {
	e.t.Helper()
	var res authResponse
	code := e.do("POST", "/api/auth/register", "", map[string]string{
		"email": email, "password": "secret123", "full_name": "Тест Пайдаланушы",
	}, &res)
	if code != 201 {
		e.t.Fatalf("register %s: %d", email, code)
	}
	return res.Token
}

func (e *testEnv) stock(productID int) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(context.Background(), "SELECT stock FROM products WHERE id = $1", productID).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

var order = map[string]string{"phone": "+77001112233", "shipping_address": "Алматы, Абай 1"}

func TestAuth(t *testing.T) {
	e := setup(t)
	var errResp map[string]any

	if code := e.do("POST", "/api/auth/login", "", map[string]string{"email": "aruzhan@mail.kz", "password": "wrong"}, &errResp); code != 401 {
		t.Errorf("қате құпиясөз: күтілді 401, алынды %d", code)
	}
	if errResp["error"] == nil {
		t.Error(`қате жауабы { "error": "..." } форматында болуы керек`)
	}
	token := e.register("new@mail.kz")
	if code := e.do("POST", "/api/auth/register", "", map[string]string{
		"email": "NEW@mail.kz", "password": "secret123", "full_name": "Қайталау",
	}, nil); code != 409 {
		t.Errorf("қайталанған email: күтілді 409, алынды %d", code)
	}
	var me User
	if code := e.do("GET", "/api/me", token, nil, &me); code != 200 || me.Email != "new@mail.kz" || me.Role != "customer" {
		t.Errorf("/api/me: %d %+v", code, me)
	}
	if code := e.do("GET", "/api/cart", "", nil, nil); code != 401 {
		t.Errorf("токенсіз себет: күтілді 401, алынды %d", code)
	}
	if code := e.do("GET", "/api/cart", "bad.token.value", nil, nil); code != 401 {
		t.Errorf("жарамсыз токен: күтілді 401, алынды %d", code)
	}
	if code := e.do("GET", "/api/admin/orders", token, nil, nil); code != 403 {
		t.Errorf("сатып алушы әкімші бетіне: күтілді 403, алынды %d", code)
	}
}

func TestCatalog(t *testing.T) {
	e := setup(t)
	var page ProductPage

	e.do("GET", "/api/products", "", nil, &page)
	if page.Total != 8 {
		t.Errorf("барлық тауар: күтілді 8, алынды %d", page.Total)
	}
	e.do("GET", "/api/products?q=apple", "", nil, &page)
	if page.Total != 3 {
		t.Errorf("'apple' іздеу: күтілді 3, алынды %d", page.Total)
	}
	// «Аксессуарлар» (3) ішкі санатымен (4) бірге: 1 + 2 = 3 тауар
	e.do("GET", "/api/products?category=3", "", nil, &page)
	if page.Total != 3 {
		t.Errorf("санат + ішкі санаттар: күтілді 3, алынды %d", page.Total)
	}
	e.do("GET", "/api/products?sort=price_asc&limit=1", "", nil, &page)
	if len(page.Items) != 1 || page.Items[0].Price != 19990 || page.Pages != 8 {
		t.Errorf("ең арзан тауар / беттер: %+v", page)
	}
	e.do("GET", "/api/products?min_price=100000&max_price=200000", "", nil, &page)
	for _, p := range page.Items {
		if p.Price < 100000 || p.Price > 200000 {
			t.Errorf("баға сүзгісінен тыс: %s %.0f", p.Name, p.Price)
		}
	}
	var details ProductDetails
	if code := e.do("GET", "/api/products/7", "", nil, &details); code != 200 || len(details.Reviews) != 1 || details.AvgRating == nil {
		t.Errorf("тауар беті: %d %+v", code, details)
	}
	if code := e.do("GET", "/api/products/999", "", nil, nil); code != 404 {
		t.Errorf("жоқ тауар: күтілді 404, алынды %d", code)
	}
	if code := e.do("GET", "/api/products?sort=evil", "", nil, nil); code != 400 {
		t.Errorf("қате sort: күтілді 400, алынды %d", code)
	}
}

func TestOrderTransaction(t *testing.T) {
	e := setup(t)
	token := e.login("aruzhan@mail.kz", "password123") // себетінде: тауар 6 × 2, тауар 3 × 1

	before6, before3 := e.stock(6), e.stock(3)
	var o Order
	if code := e.do("POST", "/api/orders", token, order, &o); code != 201 {
		t.Fatalf("тапсырыс: %d", code)
	}
	// 2×19990 + 119990 = 159970 ≥ 50000 → жеткізу тегін
	if o.Total != 159970 || o.DeliveryCost != 0 || len(o.Items) != 2 || o.Status != "new" {
		t.Errorf("тапсырыс мазмұны қате: %+v", o)
	}
	if e.stock(6) != before6-2 || e.stock(3) != before3-1 {
		t.Error("қойма қалдығы азаймады")
	}
	var cart Cart
	e.do("GET", "/api/cart", token, nil, &cart)
	if len(cart.Items) != 0 {
		t.Error("тапсырыстан кейін себет тазаланбады")
	}
	if code := e.do("POST", "/api/orders", token, order, nil); code != 400 {
		t.Errorf("бос себет: күтілді 400, алынды %d", code)
	}

	// Арзан тапсырыс → курьерлік жеткізу ақылы
	e.do("POST", "/api/cart", token, map[string]int{"product_id": 8, "quantity": 1}, nil)
	e.do("POST", "/api/orders", token, order, &o)
	if o.DeliveryCost != CourierCost || o.Total != 24990+CourierCost {
		t.Errorf("жеткізу құны: %+v", o)
	}

	// Болдырмау → қалдық қайтарылады; қайта болдырмау мүмкін емес
	if code := e.do("POST", fmt.Sprintf("/api/orders/%d/cancel", o.ID), token, nil, &o); code != 200 || o.Status != "cancelled" {
		t.Errorf("болдырмау: %d %s", code, o.Status)
	}
	if e.stock(8) != 1 {
		t.Errorf("болдырмағаннан кейін қалдық қайтарылмады: %d", e.stock(8))
	}
	other := e.login("daniyar@mail.kz", "password123")
	if code := e.do("POST", fmt.Sprintf("/api/orders/%d/cancel", o.ID), other, nil, nil); code != 404 {
		t.Errorf("бөтен тапсырыс: күтілді 404, алынды %d", code)
	}
}

func TestOrderRollbackOnInsufficientStock(t *testing.T) {
	e := setup(t)
	token := e.login("aruzhan@mail.kz", "password123")
	// Қалдықты әдейі азайтамыз: себетте 2 дана, қоймада 1
	e.db.Exec(context.Background(), "UPDATE products SET stock = 1 WHERE id = 6")
	stock3 := e.stock(3)

	var resp map[string]string
	if code := e.do("POST", "/api/orders", token, order, &resp); code != 400 {
		t.Fatalf("күтілді 400, алынды %d", code)
	}
	var orders int
	e.db.QueryRow(context.Background(), "SELECT COUNT(*) FROM orders WHERE user_id = 2").Scan(&orders)
	var cart Cart
	e.do("GET", "/api/cart", token, nil, &cart)
	if orders != 0 || e.stock(3) != stock3 || len(cart.Items) != 2 {
		t.Errorf("ROLLBACK болмады: orders=%d stock3=%d cart=%d", orders, e.stock(3), len(cart.Items))
	}
}

// Екі сатып алушы соңғы тауарды бір мезгілде алғысы келеді — тек біреуі сәтті болуы керек.
func TestConcurrentLastItem(t *testing.T) {
	e := setup(t)
	const buyers = 5
	tokens := make([]string, buyers)
	for i := range tokens {
		tokens[i] = e.register(fmt.Sprintf("buyer%d@mail.kz", i))
		// JBL Tune 520BT (id=8) — қоймада 1 дана
		if code := e.do("POST", "/api/cart", tokens[i], map[string]int{"product_id": 8, "quantity": 1}, nil); code != 201 {
			t.Fatalf("себетке қосу: %d", code)
		}
	}

	var wg sync.WaitGroup
	codes := make(chan int, buyers)
	for _, tok := range tokens {
		wg.Add(1)
		go func(tok string) {
			defer wg.Done()
			codes <- e.do("POST", "/api/orders", tok, order, nil)
		}(tok)
	}
	wg.Wait()
	close(codes)

	success := 0
	for c := range codes {
		if c == 201 {
			success++
		} else if c != 400 {
			t.Errorf("күтпеген статус: %d", c)
		}
	}
	if success != 1 || e.stock(8) != 0 {
		t.Errorf("сәтті тапсырыс: %d (күтілді 1), қалдық: %d (күтілді 0)", success, e.stock(8))
	}
}

func TestAdmin(t *testing.T) {
	e := setup(t)
	admin := e.login("admin@shop.kz", "admin123")

	var p Product
	code := e.do("POST", "/api/admin/products", admin, map[string]any{
		"name": "Тест тауары", "price": 999.999, "stock": 5, "category_id": 1,
	}, &p)
	if code != 201 || p.Price != 1000 || !p.IsActive {
		t.Fatalf("тауар қосу: %d %+v", code, p)
	}
	if code := e.do("POST", "/api/admin/products", admin, map[string]any{"name": "X", "price": -1}, nil); code != 400 {
		t.Errorf("қате тауар: күтілді 400, алынды %d", code)
	}
	if code := e.do("POST", "/api/admin/products", admin, map[string]any{"name": "Жоқ санат", "price": 1, "category_id": 999}, nil); code != 400 {
		t.Errorf("жоқ санат: күтілді 400, алынды %d", code)
	}

	// Жасыру → каталогта көрінбейді
	e.do("PUT", fmt.Sprintf("/api/admin/products/%d", p.ID), admin, map[string]any{
		"name": p.Name, "price": p.Price, "stock": p.Stock, "category_id": 1, "is_active": false,
	}, &p)
	if code := e.do("GET", fmt.Sprintf("/api/products/%d", p.ID), "", nil, nil); code != 404 {
		t.Errorf("жасырылған тауар: күтілді 404, алынды %d", code)
	}

	// Тапсырыс күйін өзгерту
	var o Order
	if code := e.do("PUT", "/api/admin/orders/1/status", admin, map[string]string{"status": "shipped"}, &o); code != 200 || o.Status != "shipped" {
		t.Errorf("күй өзгерту: %d %s", code, o.Status)
	}
	if code := e.do("PUT", "/api/admin/orders/1/status", admin, map[string]string{"status": "lost"}, nil); code != 400 {
		t.Errorf("қате күй: күтілді 400, алынды %d", code)
	}
	var st adminStats
	if code := e.do("GET", "/api/admin/stats", admin, nil, &st); code != 200 || st.Users != 3 {
		t.Errorf("статистика: %d %+v", code, st)
	}
}

func TestReviews(t *testing.T) {
	e := setup(t)
	token := e.login("aruzhan@mail.kz", "password123")

	if code := e.do("POST", "/api/products/1/reviews", token, map[string]any{"rating": 6}, nil); code != 400 {
		t.Errorf("баға 6: күтілді 400, алынды %d", code)
	}
	var rv Review
	e.do("POST", "/api/products/1/reviews", token, map[string]any{"rating": 4, "comment": "Жақсы"}, &rv)
	e.do("POST", "/api/products/1/reviews", token, map[string]any{"rating": 2, "comment": "Ойымды өзгерттім"}, &rv)
	var d ProductDetails
	e.do("GET", "/api/products/1", "", nil, &d)
	if len(d.Reviews) != 1 || d.Reviews[0].Rating != 2 {
		t.Errorf("бір тауарға бір пікір болуы керек: %+v", d.Reviews)
	}
	other := e.login("daniyar@mail.kz", "password123")
	if code := e.do("DELETE", fmt.Sprintf("/api/reviews/%d", rv.ID), other, nil, nil); code != 403 {
		t.Errorf("бөтен пікірді жою: күтілді 403, алынды %d", code)
	}
	if code := e.do("DELETE", fmt.Sprintf("/api/reviews/%d", rv.ID), token, nil, nil); code != 204 {
		t.Errorf("өз пікірін жою: күтілді 204, алынды %d", code)
	}
}
