// Интернет-дүкен (электроника және аксессуарлар) — Go бэкенді.
//
// Іске қосу:  go run .
// Тексеру:    http://localhost:3000/api/products
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"shop/internal/api"
	"shop/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("баптау қатесі: %v", err)
	}

	ctx := context.Background()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("DATABASE_URL қате: %v", err)
	}
	// Бос тұрған қосылыстарды тез жабамыз: бұлттағы тегін дерекқор (Neon)
	// пайдаланушы жоқта «ұйықтап», тегін лимитті үнемдей алады.
	poolCfg.MaxConnIdleTime = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Fatalf("дерекқор баптауы қате: %v", err)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		log.Fatalf("PostgreSQL-ге қосылу мүмкін болмады: %v\n"+
			"→ PostgreSQL іске қосылғанын, shop дерекқоры барын және .env ішіндегі DB_PASSWORD дұрыс екенін тексеріңіз", err)
	}
	log.Println("PostgreSQL-ге қосылды")

	if err := api.EnsureSchema(ctx, pool, cfg.SchemaFile); err != nil {
		log.Fatalf("дерекқорды дайындау қатесі: %v", err)
	}
	if cfg.AdminPassword != "" {
		if err := api.SetAdminPassword(ctx, pool, cfg.AdminPassword); err != nil {
			log.Fatalf("ADMIN_PASSWORD: %v", err)
		}
	}

	srv := api.NewServer(pool, cfg.JWTSecret, cfg.JWTTTL)

	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Routes())
	// Фронтенд файлдары (index.html, style.css, app.js ...) осы сервер арқылы беріледі
	if st, err := os.Stat(cfg.FrontendDir); err == nil && st.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(cfg.FrontendDir)))
		log.Printf("фронтенд: %s", cfg.FrontendDir)
	} else {
		log.Printf("ескерту: фронтенд қалтасы табылмады (%s) — тек API жұмыс істейді", cfg.FrontendDir)
	}

	handler := api.Recoverer(api.Logger(api.CORS(cfg.CORSOrigin, mux)))
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Ctrl+C басқанда сервер ағымдағы сұраныстарды аяқтап барып тоқтайды
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Println("сервер тоқтатылуда...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("сервер іске қосылды: http://localhost:%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("сервер қатесі: %v", err)
	}
}
