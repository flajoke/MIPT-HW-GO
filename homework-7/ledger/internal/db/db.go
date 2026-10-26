package db

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net"
	"net/url"
	"os"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func DSN() string {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword(env("DB_USER", "postgres"), env("DB_PASS", "postgres")), Host: net.JoinHostPort(env("DB_HOST", "localhost"), env("DB_PORT", "5432")), Path: "/" + env("DB_NAME", "cashapp")}
	query := u.Query()
	query.Set("sslmode", "disable")
	u.RawQuery = query.Encode()
	return u.String()
}
func Open(ctx context.Context) (*sql.DB, error) {
	database, err := sql.Open("pgx", DSN())
	if err != nil {
		return nil, fmt.Errorf("PostgreSQL: неверная конфигурация подключения: %w", err)
	}
	database.SetMaxOpenConns(10)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(30 * time.Minute)
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("PostgreSQL недоступен; проверьте DATABASE_URL и запуск БД: %w", err)
	}
	return database, nil
}
