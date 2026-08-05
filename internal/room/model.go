package room

import "time"

type Type int

const (
	TypeFriend Type = 1
	TypeGroup  Type = 2
)

type HotFlag int

const (
	HotNo  HotFlag = 0
	HotYes HotFlag = 1
)

type Role int

const (
	RoleOwner  Role = 1
	RoleAdmin  Role = 2
	RoleMember Role = 3
)

type Room struct {
	ID         int64     `db:"id"`
	Type       Type      `db:"type"`
	HotFlag    HotFlag   `db:"hot_flag"`
	ActiveTime time.Time `db:"active_time"`
	LastMsgID  int64     `db:"last_msg_id"`
	CreateTime time.Time `db:"create_time"`
}

func (r Room) IsHot() bool   { return r.HotFlag == HotYes }
func (r Room) IsGroup() bool { return r.Type == TypeGroup }

type Group struct {
	ID     int64  `db:"id"`
	RoomID int64  `db:"room_id"`
	Name   string `db:"name"`
	Avatar string `db:"avatar"`
}

type Friend struct {
	ID     int64 `db:"id"`
	RoomID int64 `db:"room_id"`
	UID1   int64 `db:"uid1"`
	UID2   int64 `db:"uid2"`
}

type Member struct {
	ID         int64     `db:"id"`
	GroupID    int64     `db:"group_id"`
	UID        int64     `db:"uid"`
	Role       Role      `db:"role"`
	CreateTime time.Time `db:"create_time"`
}
