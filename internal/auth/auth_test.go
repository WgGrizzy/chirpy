package auth

import "testing"

func TestHash(t *testing.T) {
	password := "password1$"
	hash, err := HashPassword(password)
	if err != nil {
		t.Error(err)
	}
	t.Log(hash)
}

func TestCompare(t *testing.T) {
	password := "password1$"
	hash, err := HashPassword(password)
	if err != nil {
		t.Error(err)
	}
	match, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%v", match)
}
