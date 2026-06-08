package onvif

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewWSUsernameToken(t *testing.T) {
	token := NewWSUsernameToken("admin", "password123")

	if token.Username != "admin" {
		t.Errorf("expected admin, got %s", token.Username)
	}

	if token.Nonce == "" {
		t.Error("nonce should not be empty")
	}

	if token.Created == "" {
		t.Error("created should not be empty")
	}

	if token.PasswordDigest == "" {
		t.Error("password digest should not be empty")
	}
}

func TestWSUsernameTokenConsistency(t *testing.T) {
	token1 := NewWSUsernameToken("admin", "password123")
	token2 := NewWSUsernameToken("admin", "password123")

	if token1.Nonce == token2.Nonce {
		t.Error("nonces should be different for each token (randomized)")
	}

	if token1.PasswordDigest == token2.PasswordDigest {
		t.Error("password digests should differ (nonce-based)")
	}
}

func TestWSUsernameTokenSOAPHeader(t *testing.T) {
	token := NewWSUsernameToken("admin", "password123")
	header := token.SOAPHeader()

	if !strings.Contains(header, "admin") {
		t.Error("header should contain username")
	}

	if !strings.Contains(header, token.PasswordDigest) {
		t.Error("header should contain password digest")
	}

	if !strings.Contains(header, token.Nonce) {
		t.Error("header should contain nonce")
	}

	if !strings.Contains(header, token.Created) {
		t.Error("header should contain created timestamp")
	}

	if !strings.Contains(header, "wsse:Security") {
		t.Error("header should contain wsse:Security element")
	}
}

func TestApplyAuthNoCredentials(t *testing.T) {
	req := createTestRequest()
	ApplyAuth(req, nil)

	if _, ok := req.Header["Authorization"]; ok {
		t.Error("no authorization header should be set with nil credentials")
	}
}

func TestApplyAuthEmptyCredentials(t *testing.T) {
	req := createTestRequest()
	ApplyAuth(req, &Credentials{})

	if _, ok := req.Header["Authorization"]; ok {
		t.Error("no authorization header should be set with empty credentials")
	}
}

func TestApplyAuthBasic(t *testing.T) {
	req := createTestRequest()
	ApplyAuth(req, &Credentials{Username: "admin", Password: "password123"})

	if _, ok := req.Header["Authorization"]; !ok {
		t.Error("authorization header should be set")
	}
}

func createTestRequest() *http.Request {
	req, _ := http.NewRequest("POST", "http://test.com", nil)
	return req
}
