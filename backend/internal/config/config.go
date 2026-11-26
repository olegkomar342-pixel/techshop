// Package config қосымша баптауларын орта айнымалыларынан (.env) оқиды.
package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	JWTTTL      time.Duration
	FrontendDir string
	CORSOrigin  string
	// SchemaFile — кестелер әлі жоқ болса, бірінші іске қосуда орындалатын SQL файлы.
	SchemaFile string
	// AdminPassword берілсе, admin@shop.kz құпиясөзі іске қосылған сайын осыған ауыстырылады.
	AdminPassword string
}

// Load .env файлын (бар болса) жүктеп, баптауларды қайтарады.
func Load() (*Config, error) {
	// .env міндетті емес: серверде нақты орта айнымалылары қолданылады
	if err := godotenv.Load(); err != nil && os.Getenv("DATABASE_URL") == "" {
		log.Printf("ескерту: .env файлы оқылмады (%v). Windows-та файл .env.txt болып сақталмағанын тексеріңіз", err)
	}

	cfg := &Config{
		Port:          env("PORT", "3000"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		FrontendDir:   env("FRONTEND_DIR", "../frontend"),
		CORSOrigin:    env("CORS_ORIGIN", ""),
		SchemaFile:    env("SCHEMA_FILE", "../database/01_schema.sql"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}

	ttl, err := time.ParseDuration(env("JWT_TTL", "168h"))
	if err != nil {
		return nil, fmt.Errorf("JWT_TTL қате: %w", err)
	}
	cfg.JWTTTL = ttl

	// DATABASE_URL берілсе — соны қолданамыз, әйтпесе DB_* айнымалыларынан құрамыз.
	cfg.DatabaseURL = cleanDatabaseURL(os.Getenv("DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		u := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(env("DB_USER", "postgres"), os.Getenv("DB_PASSWORD")),
			Host:   env("DB_HOST", "localhost") + ":" + env("DB_PORT", "5432"),
			Path:   "/" + env("DB_NAME", "shop"),
		}
		q := u.Query()
		q.Set("sslmode", env("DB_SSLMODE", "disable"))
		u.RawQuery = q.Encode()
		cfg.DatabaseURL = u.String()
	}

	if os.Getenv("DATABASE_URL") == "" {
		switch os.Getenv("DB_PASSWORD") {
		case "":
			return nil, fmt.Errorf("DB_PASSWORD бос: .env файлына PostgreSQL құпиясөзін жазыңыз")
		case "your_postgres_password":
			return nil, fmt.Errorf("DB_PASSWORD өзгертілмеген: .env ішінде your_postgres_password орнына өз құпиясөзіңізді жазыңыз")
		}
	}
	if len(cfg.JWTSecret) < 16 {
		return nil, fmt.Errorf("JWT_SECRET кемінде 16 таңбадан тұруы керек (.env файлын тексеріңіз)")
	}
	return cfg, nil
}

// cleanDatabaseURL pgx драйвері танымайтын параметрлерді алып тастайды.
// Мысалы, Neon беретін сілтемеде channel_binding=require бар — pgx оны серверге
// баптау ретінде жібереді де, қосылу сәтсіз аяқталады.
func cleanDatabaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Del("channel_binding")
	u.RawQuery = q.Encode()
	return u.String()
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
