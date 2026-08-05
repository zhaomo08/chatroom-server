package auth

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type User struct {
	ID           int64     `db:"id"`
	Username     string    `db:"username"`
	PasswordHash string    `db:"password_hash"`
	Nickname     string    `db:"nickname"`
	Avatar       string    `db:"avatar"`
	CreateTime   time.Time `db:"create_time"`
}

type Store interface {
	CreateUser(ctx context.Context, username, passwordHash, nickname string) (int64, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
}

type SQLStore struct{ db *sqlx.DB }

func NewSQLStore(db *sqlx.DB) *SQLStore { return &SQLStore{db: db} }

func (s *SQLStore) CreateUser(ctx context.Context, username, passwordHash, nickname string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO user (username, password_hash, nickname) VALUES (?, ?, ?)`,
		username, passwordHash, nickname)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.db.GetContext(ctx, &u, `SELECT * FROM user WHERE username = ?`, username)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
