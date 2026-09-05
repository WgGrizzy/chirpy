package auth

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

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

func TestMakeAndValidateJWT(t *testing.T) {
	id := uuid.New()
	t.Logf("User Id: %s", id)
	tokenSecret := "MySecretKey"
	expire := 10 * time.Minute
	signed, err := MakeJWT(id, tokenSecret, expire)
	if err != nil {
		t.Error(err)
	}
	t.Logf("Signed Token: %s", signed)

	validateId, err := ValidateJWT(signed, tokenSecret)
	if err != nil {
		t.Error(err)
	}
	t.Logf("Validate User ID: %s", validateId.String())

}

func TestGetBearerToken(t *testing.T) {
	headers := http.Header{}
	headers.Add("Authorization", "Bearer abcdefg")
	token, err := GetBearerToken(headers)
	if err != nil {
		t.Error(err)
	}
	t.Log(token)
}

func TestTest(t *testing.T) {
	type Dee struct {
		User_ID string
	}
	type Bee struct {
		Event string
		Data  Dee
	}
	bee := Bee{
		Event: "abc",
		Data: Dee{
			User_ID: "def",
		},
	}
	d, _ := json.Marshal(bee)
	t.Log(string(d))

}
