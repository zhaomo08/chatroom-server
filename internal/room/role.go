package room

import "context"

// IsParticipant reports whether uid may interact with room rm — post a
// message into it, or join a call in it. Hot rooms admit any authenticated
// user; ordinary groups require membership; 1:1 rooms require being one of
// the two participants.
func IsParticipant(ctx context.Context, store Store, rm Room, uid int64) bool {
	if rm.IsHot() {
		return true
	}
	if rm.IsGroup() {
		_, err := store.GetMember(ctx, rm.ID, uid)
		return err == nil
	}
	friend, err := store.GetFriendByRoomID(ctx, rm.ID)
	if err != nil {
		return false
	}
	return friend.UID1 == uid || friend.UID2 == uid
}

// CanRemoveMember reports whether actor may remove target from the group.
// The owner can remove anyone except itself; an admin can only remove
// ordinary members; ordinary members can't remove anyone.
func CanRemoveMember(actor, target Role) bool {
	switch actor {
	case RoleOwner:
		return target != RoleOwner
	case RoleAdmin:
		return target == RoleMember
	default:
		return false
	}
}

// CanSetAdmin reports whether actor may promote/revoke admins. Only the owner can.
func CanSetAdmin(actor Role) bool {
	return actor == RoleOwner
}

// CanDismissGroup reports whether actor may dissolve the group. Only the owner can.
func CanDismissGroup(actor Role) bool {
	return actor == RoleOwner
}
