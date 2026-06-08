package onvif

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"
)

type Credentials struct {
	Username string
	Password string
}

type WSUsernameToken struct {
	Username  string
	Password  string
	Nonce     string
	Created   string
	PasswordDigest string
}

func NewWSUsernameToken(username, password string) *WSUsernameToken {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)
	created := time.Now().UTC().Format(time.RFC3339)

	hash := sha1.New()
	hash.Write([]byte(nonceB64))
	hash.Write([]byte(created))
	hash.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	return &WSUsernameToken{
		Username:       username,
		Password:       password,
		Nonce:          nonceB64,
		Created:        created,
		PasswordDigest: digest,
	}
}

func (t *WSUsernameToken) SOAPHeader() string {
	return fmt.Sprintf(`<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
    <wsse:UsernameToken wsu:Id="UsernameToken-1">
      <wsse:Username>%s</wsse:Username>
      <wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
      <wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
      <wsu:Created>%s</wsu:Created>
    </wsse:UsernameToken>
  </wsse:Security>`, t.Username, t.PasswordDigest, t.Nonce, t.Created)
}

func ApplyAuth(req *http.Request, creds *Credentials) {
	if creds == nil || creds.Username == "" {
		return
	}
	req.SetBasicAuth(creds.Username, creds.Password)
}
