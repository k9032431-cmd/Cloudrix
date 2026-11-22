package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Sorting puts "no limit" and "no end date" last when looking for the least.
func TestUserSorting(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	admin := &model.Admin{Username: "root", PasswordHash: "x", Role: model.RoleSudo}
	if err := s.CreateAdmin(ctx, admin); err != nil {
		t.Fatal(err)
	}
	day := func(n int) *time.Time { v := time.Now().Add(time.Duration(n) * 24 * time.Hour); return &v }
	add := func(name string, limit, used int64, exp *time.Time) {
		u := &model.User{AdminID: admin.ID, Username: name, Status: model.StatusActive, SubToken: "token-" + name + "-0123456789", Credentials: model.Credentials{UUID: "uuid-" + name, Password: "pw-" + name}, DataLimit: limit, UsedTraffic: used, ExpireAt: exp}
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if used > 0 {
			if err := s.AddTraffic(ctx, u.ID, used); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("bob", 100, 90, day(30))  // 10 left, 30 days
	add("Alice", 0, 500, nil)     // no limit, forever
	add("carol", 100, 20, day(3)) // 80 left, 3 days
	order := func(sort string) string {
		list, _, err := s.ListUsers(ctx, UserFilter{Sort: sort, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, u := range list {
			names = append(names, u.Username)
		}
		return strings.Join(names, ",")
	}
	for sort, want := range map[string]string{
		"username":      "Alice,bob,carol",
		"-used_traffic": "Alice,bob,carol",
		"data_left":     "bob,carol,Alice",
		"expire_at":     "carol,bob,Alice",
		"-expire_at":    "Alice,bob,carol",
	} {
		if got := order(sort); got != want {
			t.Errorf("%s: got %s, want %s", sort, got, want)
		}
	}
}
