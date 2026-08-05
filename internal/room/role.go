package room

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
