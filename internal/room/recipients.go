package room

// Recipients returns the uids that should receive a message sent in room r.
// Callers must special-case hot rooms (r.IsHot()) themselves: those broadcast
// to every online connection instead of doing per-member fan-out.
func Recipients(r Room, groupMemberUIDs []int64, friend *Friend) []int64 {
	switch r.Type {
	case TypeGroup:
		out := make([]int64, len(groupMemberUIDs))
		copy(out, groupMemberUIDs)
		return out
	case TypeFriend:
		if friend == nil {
			return nil
		}
		return []int64{friend.UID1, friend.UID2}
	default:
		return nil
	}
}
