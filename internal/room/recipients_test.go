package room

import (
	"reflect"
	"testing"
)

func TestRecipientsGroup(t *testing.T) {
	r := Room{Type: TypeGroup}
	got := Recipients(r, []int64{1, 2, 3}, nil)
	want := []int64{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Recipients = %v, want %v", got, want)
	}
}

func TestRecipientsFriend(t *testing.T) {
	r := Room{Type: TypeFriend}
	friend := &Friend{UID1: 10, UID2: 20}
	got := Recipients(r, nil, friend)
	want := []int64{10, 20}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Recipients = %v, want %v", got, want)
	}
}

func TestRecipientsFriendMissing(t *testing.T) {
	r := Room{Type: TypeFriend}
	got := Recipients(r, nil, nil)
	if got != nil {
		t.Errorf("Recipients = %v, want nil when friend is missing", got)
	}
}
