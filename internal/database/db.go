package database

import (
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func Connect(dsn string) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		slog.Error("erro ao abrir conexão com o banco", "err", err)
		os.Exit(1)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err = db.Ping(); err != nil {
		slog.Error("banco não responde", "err", err)
		os.Exit(1)
	}

	slog.Info("conectado ao PostgreSQL")
	DB = db
}
