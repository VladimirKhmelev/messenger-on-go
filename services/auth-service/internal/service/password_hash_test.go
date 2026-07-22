package service

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordHash_SameInputProducesDifferentHashes(t *testing.T) {
	hashA, err := bcrypt.GenerateFromPassword([]byte("sixseven"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() unexpected error: %v", err)
	}
	hashB, err := bcrypt.GenerateFromPassword([]byte("sixseven"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() unexpected error: %v", err)
	}

	if string(hashA) == string(hashB) {
		t.Error("GenerateFromPassword() produced identical hashes for the same password twice, want different (salted) hashes")
	}
}

func TestPasswordHash_CorrectPasswordMatches(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("abcd1234"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() unexpected error: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword(hash, []byte("abcd1234")); err != nil {
		t.Errorf("CompareHashAndPassword() unexpected error for correct password: %v", err)
	}
}

func TestPasswordHash_WrongPasswordDoesNotMatch(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("abcd1234"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() unexpected error: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword(hash, []byte("wrongpass1")); err == nil {
		t.Error("CompareHashAndPassword() succeeded for wrong password, want error")
	}
}
