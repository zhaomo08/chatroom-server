package db

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"

	_ "github.com/go-sql-driver/mysql"
)

func Connect(dsn string) (*sqlx.DB, error) {
	return sqlx.Connect("mysql", dsn)
}

// Migrate applies all up migrations found in migrationsDir to the database at dsn.
func Migrate(dsn, migrationsDir string) error {
	m, err := migrate.New(fmt.Sprintf("file://%s", migrationsDir), fmt.Sprintf("mysql://%s", dsn))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
