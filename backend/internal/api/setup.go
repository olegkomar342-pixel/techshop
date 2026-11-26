package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// EnsureSchema дерекқор бос болса (users кестесі жоқ), схема мен сынақ деректерін жүктейді.
// Кестелер бар болса ештеңе істемейді — бар деректер ешқашан өшірілмейді.
// Бұл бұлттағы (Render, Railway) бос дерекқорды pgAdmin-сіз дайындауға мүмкіндік береді.
func EnsureSchema(ctx context.Context, db *pgxpool.Pool, schemaFile string) error {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('public.users') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	sql, err := os.ReadFile(schemaFile)
	if errors.Is(err, os.ErrNotExist) {
		log.Printf("ескерту: дерекқор бос, бірақ схема файлы табылмады (%s) — 01_schema.sql файлын қолмен орындаңыз", schemaFile)
		return nil
	}
	if err != nil {
		return err
	}
	log.Printf("дерекқор бос — %s жүктелуде...", schemaFile)
	if _, err := db.Exec(ctx, "SET client_min_messages = warning;\n"+string(sql)); err != nil {
		return fmt.Errorf("схеманы жүктеу: %w", err)
	}
	log.Println("схема және сынақ деректері жүктелді")
	return nil
}

// SetAdminPassword admin@shop.kz құпиясөзін берілген мәнге ауыстырады.
// Сайт интернетте ашық болса, сынақтағы admin123 құпиясөзін міндетті түрде ауыстыру керек.
func SetAdminPassword(ctx context.Context, db *pgxpool.Pool, password string) error {
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return fmt.Errorf("ADMIN_PASSWORD 8–72 таңбадан тұруы керек")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE email = 'admin@shop.kz'`, string(hash))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		log.Println("admin@shop.kz құпиясөзі ADMIN_PASSWORD мәніне ауыстырылды")
	}
	return nil
}
