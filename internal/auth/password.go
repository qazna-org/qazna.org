package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes plaintext password using bcrypt.
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", errors.New("password is empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword compares plaintext password with stored hash.
func VerifyPassword(hash, password string) error {
	if hash == "" {
		return errors.New("password hash is empty")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
