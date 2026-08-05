package message

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	Insert(ctx context.Context, m *Message) (int64, error)
	GetByID(ctx context.Context, id int64) (*Message, error)
	SetStatus(ctx context.Context, id int64, status Status) error
	AddMark(ctx context.Context, msgID, uid int64, markType int) error
	ListByRoomCursor(ctx context.Context, roomID, beforeID int64, limit int) ([]Message, error)
}

type SQLStore struct{ db *sqlx.DB }

func NewSQLStore(db *sqlx.DB) *SQLStore { return &SQLStore{db: db} }

func (s *SQLStore) Insert(ctx context.Context, m *Message) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO message (room_id, from_uid, content, type, reply_msg_id, status) VALUES (?, ?, ?, ?, ?, ?)`,
		m.RoomID, m.FromUID, m.Content, m.Type, m.ReplyMsgID, m.Status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLStore) GetByID(ctx context.Context, id int64) (*Message, error) {
	var m Message
	err := s.db.GetContext(ctx, &m, `SELECT * FROM message WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

func (s *SQLStore) SetStatus(ctx context.Context, id int64, status Status) error {
	_, err := s.db.ExecContext(ctx, `UPDATE message SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *SQLStore) AddMark(ctx context.Context, msgID, uid int64, markType int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO message_mark (msg_id, uid, type) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE create_time = create_time`,
		msgID, uid, markType)
	return err
}

// ListByRoomCursor returns up to limit messages in roomID with id < beforeID,
// newest first. Pass beforeID = 0 to start from the most recent message.
func (s *SQLStore) ListByRoomCursor(ctx context.Context, roomID, beforeID int64, limit int) ([]Message, error) {
	var msgs []Message
	var err error
	if beforeID == 0 {
		err = s.db.SelectContext(ctx, &msgs,
			`SELECT * FROM message WHERE room_id = ? ORDER BY id DESC LIMIT ?`, roomID, limit)
	} else {
		err = s.db.SelectContext(ctx, &msgs,
			`SELECT * FROM message WHERE room_id = ? AND id < ? ORDER BY id DESC LIMIT ?`, roomID, beforeID, limit)
	}
	return msgs, err
}
