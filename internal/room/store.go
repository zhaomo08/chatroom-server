package room

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrNotFound = errors.New("not found")

// RoomSummary is one row of a user's room list (sidebar): for a group room
// Name is the group name and PeerUID is 0; for a 1:1 room Name is empty and
// PeerUID is the other participant's uid.
type RoomSummary struct {
	RoomID        int64      `db:"room_id"`
	Type          Type       `db:"type"`
	Name          string     `db:"name"`
	PeerUID       int64      `db:"peer_uid"`
	ActiveTime    time.Time  `db:"active_time"`
	LastMessage   *string    `db:"last_message"`
	LastMessageAt *time.Time `db:"last_message_at"`
	UnreadCount   int        `db:"unread_count"`
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

	// TouchRoom bumps a room's active_time and last_msg_id after a new
	// message is inserted, so ListRoomsForUser can sort by actual recent
	// activity and show a last-message preview.
	TouchRoom(ctx context.Context, roomID, msgID int64) error
	// BumpUnread increments contact.unread_count for every uid in
	// recipientUIDs (creating the row if it doesn't exist yet).
	BumpUnread(ctx context.Context, roomID int64, recipientUIDs []int64) error
	// ResetUnread zeroes uid's unread count for roomID — called for the
	// sender right after they post (they've obviously "read" their own
	// message) and by the explicit "mark room as read" endpoint.
	ResetUnread(ctx context.Context, uid, roomID int64) error
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
	members := []Member{}
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
	out := []RoomSummary{}
	err := s.db.SelectContext(ctx, &out, `
		(SELECT r.id AS room_id, r.type AS type, g.name AS name, 0 AS peer_uid, r.active_time AS active_time,
		        lm.content AS last_message, lm.create_time AS last_message_at,
		        COALESCE(c.unread_count, 0) AS unread_count
		 FROM group_member gm
		 JOIN room r ON r.id = gm.group_id
		 JOIN room_group g ON g.room_id = r.id
		 LEFT JOIN message lm ON lm.id = r.last_msg_id
		 LEFT JOIN contact c ON c.room_id = r.id AND c.uid = gm.uid
		 WHERE gm.uid = ?)
		UNION ALL
		(SELECT r.id AS room_id, r.type AS type, '' AS name,
		        CASE WHEN f.uid1 = ? THEN f.uid2 ELSE f.uid1 END AS peer_uid,
		        r.active_time AS active_time,
		        lm.content AS last_message, lm.create_time AS last_message_at,
		        COALESCE(c.unread_count, 0) AS unread_count
		 FROM room_friend f
		 JOIN room r ON r.id = f.room_id
		 LEFT JOIN message lm ON lm.id = r.last_msg_id
		 LEFT JOIN contact c ON c.room_id = r.id AND c.uid = ?
		 WHERE f.uid1 = ? OR f.uid2 = ?)
		ORDER BY active_time DESC`, uid, uid, uid, uid, uid)
	return out, err
}

// TouchRoom bumps active_time and last_msg_id after msgID was just inserted
// into roomID. Called for every message type including hot-room broadcasts,
// so a room's recency always reflects real traffic.
func (s *SQLStore) TouchRoom(ctx context.Context, roomID, msgID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE room SET active_time = NOW(), last_msg_id = ? WHERE id = ?`, msgID, roomID)
	return err
}

func (s *SQLStore) BumpUnread(ctx context.Context, roomID int64, recipientUIDs []int64) error {
	if len(recipientUIDs) == 0 {
		return nil
	}
	values := make([]string, len(recipientUIDs))
	args := make([]any, 0, len(recipientUIDs)*2)
	for i, uid := range recipientUIDs {
		values[i] = "(?, ?, NOW(), 1)"
		args = append(args, uid, roomID)
	}
	query := `INSERT INTO contact (uid, room_id, active_time, unread_count) VALUES ` +
		strings.Join(values, ",") +
		` ON DUPLICATE KEY UPDATE active_time = NOW(), unread_count = unread_count + 1`
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *SQLStore) ResetUnread(ctx context.Context, uid, roomID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO contact (uid, room_id, active_time, unread_count) VALUES (?, ?, NOW(), 0)
		 ON DUPLICATE KEY UPDATE unread_count = 0`,
		uid, roomID)
	return err
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
