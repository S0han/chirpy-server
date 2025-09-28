package auth

import (
	"testing"
    "net/http"
    "time"
    "errors"
    "strings"
    "github.com/google/uuid"
)

func TestMakeAndValidateJWT_Success(t *testing.T) {
	id := uuid.New()
	secret := "supersecret"
	tok, err := MakeJWT(id, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT error: %v", err)
	}

	got, err := ValidateJWT(tok, secret)
	if err != nil {
		t.Fatalf("ValidateJWT error: %v", err)
	}
	if got != id {
		t.Fatalf("want %v, got %v", id, got)
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
    id := uuid.New()
    tok, err := MakeJWT(id, "right", time.Minute)
    if err != nil {
        t.Fatalf("MakeJWT error: %v", err)
    }
    if _, err := ValidateJWT(tok, "wrong"); err == nil {
        t.Fatalf("expected error for wrong secret")
    }
}

func TestValidateJWT_Expired(t *testing.T) {
    id := uuid.New()
    tok, err := MakeJWT(id, "secret", -1*time.Minute) // clearly expired
    if err != nil {
        t.Fatalf("MakeJWT error: %v", err)
    }
    if _, err := ValidateJWT(tok, "secret"); err == nil {
        t.Fatalf("expected error for expired token")
    }
}

func TestGetBearerToken_Success(t *testing.T) {
  h := http.Header{}
  h.Set("Authorization", "Bearer abc.def.ghi")
  tok, err := GetBearerToken(h)
  if err != nil {
    t.Fatalf("unexpected error: %v", err)
  }
  if tok != "abc.def.ghi" {
    t.Fatalf("got %q, want %q", tok, "abc.def.ghi")
  }
}

func TestGetBearerToken_MissingHeader(t *testing.T) {
  h := http.Header{}
  if _, err := GetBearerToken(h); err == nil {
    t.Fatal("expected error, got nil")
  }
}

func TestGetBearerToken_BadPrefix(t *testing.T) {
  h := http.Header{}
  h.Set("Authorization", "Token abc")
  if _, err := GetBearerToken(h); err == nil {
    t.Fatal("expected error, got nil")
  }
}

func TestGetBearerToken_EmptyToken(t *testing.T) {
  h := http.Header{}
  h.Set("Authorization", "Bearer   ")
  if _, err := GetBearerToken(h); err == nil {
    t.Fatal("expected error, got nil")
  }
}