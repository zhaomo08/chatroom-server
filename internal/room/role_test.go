package room

import "testing"

func TestCanRemoveMember(t *testing.T) {
	cases := []struct {
		actor, target Role
		want          bool
	}{
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleMember, true},
		{RoleOwner, RoleOwner, false},
		{RoleAdmin, RoleMember, true},
		{RoleAdmin, RoleAdmin, false},
		{RoleAdmin, RoleOwner, false},
		{RoleMember, RoleMember, false},
	}
	for _, c := range cases {
		if got := CanRemoveMember(c.actor, c.target); got != c.want {
			t.Errorf("CanRemoveMember(%v, %v) = %v, want %v", c.actor, c.target, got, c.want)
		}
	}
}

func TestCanSetAdmin(t *testing.T) {
	if !CanSetAdmin(RoleOwner) {
		t.Error("owner should be able to set admins")
	}
	if CanSetAdmin(RoleAdmin) || CanSetAdmin(RoleMember) {
		t.Error("only the owner should be able to set admins")
	}
}
