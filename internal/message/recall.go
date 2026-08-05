package message

import (
	"time"

	"chatroom-server/internal/room"
)

const recallWindow = 2 * time.Minute

// CanRecall reports whether actorUID (with the given room role, room.RoleMember
// when the actor isn't a group member e.g. in a 1:1 chat) may recall a message
// sent by fromUID at createTime, evaluated at now.
func CanRecall(fromUID, actorUID int64, actorRole room.Role, createTime, now time.Time) bool {
	if actorRole == room.RoleOwner || actorRole == room.RoleAdmin {
		return true
	}
	if fromUID != actorUID {
		return false
	}
	return now.Sub(createTime) <= recallWindow
}
