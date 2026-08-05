package message

import "time"

type Type int

const (
	TypeText   Type = 1
	TypeEmoji  Type = 2
	TypeRecall Type = 3
	TypeSystem Type = 4
)

type Status int

const (
	StatusNormal  Status = 0
	StatusDeleted Status = 1
)

type Message struct {
	ID         int64     `db:"id"`
	RoomID     int64     `db:"room_id"`
	FromUID    int64     `db:"from_uid"`
	Content    string    `db:"content"`
	Type       Type      `db:"type"`
	ReplyMsgID int64     `db:"reply_msg_id"`
	Status     Status    `db:"status"`
	CreateTime time.Time `db:"create_time"`
}

type Mark struct {
	ID         int64     `db:"id"`
	MsgID      int64     `db:"msg_id"`
	UID        int64     `db:"uid"`
	Type       int       `db:"type"`
	CreateTime time.Time `db:"create_time"`
}
