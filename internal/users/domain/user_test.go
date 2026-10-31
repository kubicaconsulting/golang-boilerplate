package users_test

import (
	"errors"
	"testing"
	"time"

	users "boilerplate/internal/users/domain"
)

func TestUserRules(t *testing.T) {
	now := time.Now()
	u, err := users.NewUser("id", " Ada ", " ADA@example.com ", now)
	if err != nil || u.Name != "Ada" || u.Email != "ada@example.com" || !u.CreatedAt.Equal(now) {
		t.Fatalf("normalization: %#v %v", u, err)
	}
	for _, input := range [][2]string{{"", "a@example.com"}, {"Ada", "invalid"}, {"Ada", "Ada <a@example.com>"}} {
		if _, createErr := users.NewUser("id", input[0], input[1], now); !errors.Is(createErr, users.ErrInvalid) {
			t.Fatalf("accepted invalid input: %v", input)
		}
	}
}
