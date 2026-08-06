package message

import "time"

type Type int

const (
	TypeText   Type = 1
	TypeEmoji  Type = 2
	TypeRecall Type = 3
	TypeSystem Type = 4
	TypeImage  Type = 5
	TypeVideo  Type = 6
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
	// Extra carries type-specific metadata as raw JSON text, e.g.
	// {"file_id":"...","thumb_id":"...","width":800,"height":600} for
	// TypeImage/TypeVideo. Unused (nil) for text/emoji/recall/system.
	Extra      *string   `db:"extra"`
	CreateTime time.Time `db:"create_time"`
}

type Mark struct {
	ID         int64     `db:"id"`
	MsgID      int64     `db:"msg_id"`
	UID        int64     `db:"uid"`
	Type       int       `db:"type"`
	CreateTime time.Time `db:"create_time"`
}
