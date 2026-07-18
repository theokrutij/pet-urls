package auth

import (
	"crypto/sha256"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(password string) (PasswordHash, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func passwordMatchesHash(password string, hash PasswordHash) bool {
	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	return err == nil
}

func hashToken(token []byte) TokenHash {
	hash := sha256.Sum256(token)
	return hash[:]
}
