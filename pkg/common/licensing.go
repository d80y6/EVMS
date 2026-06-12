package common

import (
	"crypto/ed25519"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type LicenseClaims struct {
	MaxCameras   int      `json:"max_cameras"`
	Features     []string `json:"features"`
	Tier         string   `json:"tier"`
	Issuer       string   `json:"iss"`
	ExpiresAt    int64    `json:"exp"`
	jwt.RegisteredClaims
}

type LicenseValidator struct {
	publicKey ed25519.PublicKey
}

func NewLicenseValidator(pubKey ed25519.PublicKey) *LicenseValidator {
	return &LicenseValidator{publicKey: pubKey}
}

func (v *LicenseValidator) ValidateLicense(key string) (*LicenseClaims, error) {
	token, err := jwt.ParseWithClaims(key, &LicenseClaims{}, func(t *jwt.Token) (interface{}, error) {
		return v.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid license: %w", err)
	}
	claims, ok := token.Claims.(*LicenseClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid license claims")
	}
	return claims, nil
}

func GenerateLicense(privateKey ed25519.PrivateKey, claims LicenseClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(privateKey)
}
