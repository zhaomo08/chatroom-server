package room

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	CreateGroupRoom(ctx context.Context, ownerUID int64, name, avatar string) (roomID int64, err error)
	AddMember(ctx context.Context, groupID, uid int64, role Role) error
	RemoveMember(ctx context.Context, groupID, uid int64) error
	SetRole(ctx context.Context, groupID, uid int64, role Role) error
	ListMembers(ctx context.Context, groupID int64) ([]Member, error)
	GetMember(ctx context.Context, groupID, uid int64) (*Member, error)
	GetRoom(ctx context.Context, roomID int64) (*Room, error)
	GetGroupByRoomID(ctx context.Context, roomID int64) (*Group, error)
	GetFriendByRoomID(ctx context.Context, roomID int64) (*Friend, error)
}

type SQLStore struct{ db *sqlx.DB }

func NewSQLStore(db *sqlx.DB) *SQLStore { return &SQLStore{db: db} }

func (s *SQLStore) CreateGroupRoom(ctx context.Context, ownerUID int64, name, avatar string) (int64, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT INTO room (type, hot_flag) VALUES (?, 0)`, TypeGroup)
	if err != nil {
		return 0, err
	}
	roomID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO room_group (room_id, name, avatar) VALUES (?, ?, ?)`, roomID, name, avatar); err != nil {
		return 0, err
	}

	// The design uses a 1:1 room<->room_group relationship, so room_id doubles
	// as group_member.group_id: no separate group id sequence needed.
	groupID := roomID
	if _, err := tx.ExecContext(ctx, `INSERT INTO group_member (group_id, uid, role) VALUES (?, ?, ?)`, groupID, ownerUID, RoleOwner); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return roomID, nil
}

func (s *SQLStore) AddMember(ctx context.Context, groupID, uid int64, role Role) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO group_member (group_id, uid, role) VALUES (?, ?, ?)`, groupID, uid, role)
	return err
}

func (s *SQLStore) RemoveMember(ctx context.Context, groupID, uid int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM group_member WHERE group_id = ? AND uid = ?`, groupID, uid)
	return err
}

func (s *SQLStore) SetRole(ctx context.Context, groupID, uid int64, role Role) error {
	_, err := s.db.ExecContext(ctx, `UPDATE group_member SET role = ? WHERE group_id = ? AND uid = ?`, role, groupID, uid)
	return err
}

func (s *SQLStore) ListMembers(ctx context.Context, groupID int64) ([]Member, error) {
	var members []Member
	err := s.db.SelectContext(ctx, &members, `SELECT * FROM group_member WHERE group_id = ? ORDER BY role, id`, groupID)
	return members, err
}

func (s *SQLStore) GetMember(ctx context.Context, groupID, uid int64) (*Member, error) {
	var m Member
	err := s.db.GetContext(ctx, &m, `SELECT * FROM group_member WHERE group_id = ? AND uid = ?`, groupID, uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

func (s *SQLStore) GetRoom(ctx context.Context, roomID int64) (*Room, error) {
	var r Room
	err := s.db.GetContext(ctx, &r, `SELECT * FROM room WHERE id = ?`, roomID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

func (s *SQLStore) GetGroupByRoomID(ctx context.Context, roomID int64) (*Group, error) {
	var g Group
	err := s.db.GetContext(ctx, &g, `SELECT * FROM room_group WHERE room_id = ?`, roomID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &g, err
}

func (s *SQLStore) GetFriendByRoomID(ctx context.Context, roomID int64) (*Friend, error) {
	var f Friend
	err := s.db.GetContext(ctx, &f, `SELECT * FROM room_friend WHERE room_id = ?`, roomID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}
