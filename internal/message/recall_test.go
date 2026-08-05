package message

import (
	"testing"
	"time"

	"chatroom-server/internal/room"
)

func TestCanRecallOwnMessageWithinWindow(t *testing.T) {
	now := time.Now()
	created := now.Add(-1 * time.Minute)
	if !CanRecall(5, 5, room.RoleMember, created, now) {
		t.Error("sender should be able to recall own message within the 2-minute window")
	}
}

func TestCanRecallOwnMessageTooLate(t *testing.T) {
	now := time.Now()
	created := now.Add(-10 * time.Minute)
	if CanRecall(5, 5, room.RoleMember, created, now) {
		t.Error("sender should not be able to recall own message after the 2-minute window")
	}
}

func TestCanRecallOthersMessageAsAdmin(t *testing.T) {
	now := time.Now()
	created := now.Add(-1 * time.Hour)
	if !CanRecall(5, 99, room.RoleAdmin, created, now) {
		t.Error("group admin should be able to recall anyone's message regardless of time")
	}
}

func TestCannotRecallOthersMessageAsMember(t *testing.T) {
	now := time.Now()
	created := now.Add(-30 * time.Second)
	if CanRecall(5, 99, room.RoleMember, created, now) {
		t.Error("a regular member should not be able to recall someone else's message")
	}
}
