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
	GetUsersByIDs(ctx context.Context, ids []int64) ([]User, error)
	UpdateProfile(ctx context.Context, uid int64, nickname, avatar string) error
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

// GetUsersByIDs batch-resolves display info (nickname/avatar) for a set of
// uids, e.g. to label a room list or a chat's message bubbles without one
// query per uid. Unknown ids are silently omitted from the result.
func (s *SQLStore) GetUsersByIDs(ctx context.Context, ids []int64) ([]User, error) {
	if len(ids) == 0 {
		return []User{}, nil
	}
	query, args, err := sqlx.In(`SELECT id, nickname, avatar FROM user WHERE id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	query = s.db.Rebind(query)
	users := []User{}
	if err := s.db.SelectContext(ctx, &users, query, args...); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *SQLStore) UpdateProfile(ctx context.Context, uid int64, nickname, avatar string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user SET nickname = ?, avatar = ? WHERE id = ?`, nickname, avatar, uid)
	return err
}
