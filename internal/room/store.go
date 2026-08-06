package room

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrNotFound = errors.New("not found")

// RoomSummary is one row of a user's room list (sidebar): for a group room
// Name is the group name and PeerUID is 0; for a 1:1 room Name is empty and
// PeerUID is the other participant's uid.
type RoomSummary struct {
	RoomID     int64     `db:"room_id"`
	Type       Type      `db:"type"`
	Name       string    `db:"name"`
	PeerUID    int64     `db:"peer_uid"`
	ActiveTime time.Time `db:"active_time"`
}

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
	ListRoomsForUser(ctx context.Context, uid int64) ([]RoomSummary, error)
	GetOrCreateFriendRoom(ctx context.Context, uid1, uid2 int64) (roomID int64, err error)
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

func (s *SQLStore) ListRoomsForUser(ctx context.Context, uid int64) ([]RoomSummary, error) {
	var out []RoomSummary
	err := s.db.SelectContext(ctx, &out, `
		(SELECT r.id AS room_id, r.type AS type, g.name AS name, 0 AS peer_uid, r.active_time AS active_time
		 FROM group_member gm
		 JOIN room r ON r.id = gm.group_id
		 JOIN room_group g ON g.room_id = r.id
		 WHERE gm.uid = ?)
		UNION ALL
		(SELECT r.id AS room_id, r.type AS type, '' AS name,
		        CASE WHEN f.uid1 = ? THEN f.uid2 ELSE f.uid1 END AS peer_uid,
		        r.active_time AS active_time
		 FROM room_friend f
		 JOIN room r ON r.id = f.room_id
		 WHERE f.uid1 = ? OR f.uid2 = ?)
		ORDER BY active_time DESC`, uid, uid, uid, uid)
	return out, err
}

func (s *SQLStore) GetOrCreateFriendRoom(ctx context.Context, uid1, uid2 int64) (int64, error) {
	if uid1 == uid2 {
		return 0, errors.New("cannot start a DM with yourself")
	}
	if uid1 > uid2 {
		uid1, uid2 = uid2, uid1
	}

	var existing Friend
	err := s.db.GetContext(ctx, &existing, `SELECT * FROM room_friend WHERE uid1 = ? AND uid2 = ?`, uid1, uid2)
	if err == nil {
		return existing.RoomID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT INTO room (type, hot_flag) VALUES (?, 0)`, TypeFriend)
	if err != nil {
		return 0, err
	}
	roomID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO room_friend (room_id, uid1, uid2) VALUES (?, ?, ?)`, roomID, uid1, uid2); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return roomID, nil
}
