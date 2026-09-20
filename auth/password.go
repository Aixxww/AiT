package auth

import "golang.org/x/crypto/bcrypt"

// DefaultBcryptCost is the bcrypt cost used for password hashing.
const DefaultBcryptCost = bcrypt.DefaultCost

// HashPassword hashes the password with bcrypt.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword verifies password against a bcrypt hash.
func CheckPassword(password, hash string) bool {
	if password == "" || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
