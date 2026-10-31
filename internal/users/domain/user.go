// Package users contains technology-independent user rules.
package users

import (
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalid indicates that user input violates a domain invariant.
	ErrInvalid = errors.New("invalid user")
	// ErrNotFound indicates that the requested user does not exist.
	ErrNotFound = errors.New("user not found")
	// ErrConflict indicates that a normalized email already belongs to a user.
	ErrConflict = errors.New("user already exists")
)

// User is a normalized user entity independent of storage and transport.
type User struct {
	ID        string
	Name      string
	Email     string
	CreatedAt time.Time
}

// NewUser validates and normalizes a user before persistence.
func NewUser(id, name, email string, now time.Time) (User, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if id == "" ||
		name == "" ||
		utf8.RuneCountInString(name) > 100 ||
		len(email) > 254 ||
		err != nil ||
		parsed.Address != email {
		return User{}, ErrInvalid
	}
	return User{ID: id, Name: name, Email: email, CreatedAt: now.UTC()}, nil
}
