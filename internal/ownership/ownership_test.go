package ownership

import (
	"testing"
)

func TestUserRoleFromDBRole(t *testing.T) {
	cases := []struct {
		input string
		want  UserRole
	}{
		{"owner", RoleOwner},
		{"OWNER", RoleOwner},
		{"admin", RoleAdmin},
		{"ADMIN", RoleAdmin},
		{"regular", RoleRegular},
		{"member", RoleRegular},
		{"unknown", RoleRegular},
		{"", RoleRegular},
	}
	for _, tc := range cases {
		got := FromDBRole(tc.input)
		if got != tc.want {
			t.Errorf("FromDBRole(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestUserRoleDBRoundTrip(t *testing.T) {
	for _, role := range []UserRole{RoleOwner, RoleAdmin, RoleRegular} {
		if got := FromDBRole(role.AsDBRole()); got != role {
			t.Errorf("round-trip %v failed", role)
		}
	}
}

func TestUserRolePredicates(t *testing.T) {
	if !RoleOwner.IsOwner() {
		t.Error("Owner should be owner")
	}
	if !RoleOwner.IsAdmin() {
		t.Error("Owner should be admin")
	}
	if RoleOwner.IsRegular() {
		t.Error("Owner should not be regular")
	}

	if RoleAdmin.IsOwner() {
		t.Error("Admin should not be owner")
	}
	if !RoleAdmin.IsAdmin() {
		t.Error("Admin should be admin")
	}

	if RoleRegular.IsAdmin() {
		t.Error("Regular should not be admin")
	}
	if !RoleRegular.IsRegular() {
		t.Error("Regular should be regular")
	}
}

func TestUserIDNew(t *testing.T) {
	uid, err := New("alice", RoleRegular)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if uid.AsStr() != "alice" {
		t.Errorf("id = %q", uid.AsStr())
	}
	if uid.Role() != RoleRegular {
		t.Errorf("role = %v", uid.Role())
	}
}

func TestUserIDNewRejectsEmpty(t *testing.T) {
	_, err := New("", RoleRegular)
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestUserIDNewRejectsWhitespace(t *testing.T) {
	_, err := New("   ", RoleRegular)
	if err == nil {
		t.Error("expected error for whitespace-only id")
	}
}

func TestUserIDFromTrusted(t *testing.T) {
	uid := FromTrusted("", RoleRegular)
	if uid.AsStr() != "" {
		t.Error("FromTrusted should skip validation")
	}
}

func TestUserIDString(t *testing.T) {
	uid := FromTrusted("alice", RoleAdmin)
	if uid.String() != "alice" {
		t.Errorf("String() = %q", uid.String())
	}
}

func TestUserIDEqual(t *testing.T) {
	a := FromTrusted("alice", RoleRegular)
	b := FromTrusted("alice", RoleAdmin)
	c := FromTrusted("bob", RoleRegular)

	if !a.Equal(b) {
		t.Error("same id should be equal regardless of role")
	}
	if a.Equal(c) {
		t.Error("different id should not be equal")
	}
	if a.Equal(nil) {
		t.Error("nil should not be equal")
	}
}

func TestUserIDRolePredicates(t *testing.T) {
	owner := FromTrusted("o", RoleOwner)
	if !owner.IsOwner() || !owner.IsAdmin() {
		t.Error("owner predicates failed")
	}

	reg := FromTrusted("r", RoleRegular)
	if !reg.IsRegular() || reg.IsAdmin() {
		t.Error("regular predicates failed")
	}
}

func TestBaseOwned(t *testing.T) {
	res := &BaseOwned{UserID: "alice"}
	if res.OwnerUserID() != "alice" {
		t.Errorf("owner = %q", res.OwnerUserID())
	}
	if !res.IsOwnedBy("alice") {
		t.Error("expected alice to own resource")
	}
	if res.IsOwnedBy("bob") {
		t.Error("expected bob not to own resource")
	}
}

func TestCache(t *testing.T) {
	c := NewCache()
	uid := FromTrusted("user1", RoleRegular)

	c.Set("repl", "ext1", uid)
	if c.Count() != 1 {
		t.Errorf("count = %d, want 1", c.Count())
	}

	entry, ok := c.Get("repl", "ext1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if entry.UserID.AsStr() != "user1" {
		t.Errorf("cached user = %q", entry.UserID.AsStr())
	}

	_, ok = c.Get("repl", "missing")
	if ok {
		t.Error("expected cache miss")
	}

	c.Remove("repl", "ext1")
	if c.Count() != 0 {
		t.Errorf("count = %d, want 0", c.Count())
	}
}
