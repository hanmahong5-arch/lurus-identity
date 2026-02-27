package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testRSAKey generates a small RSA key suitable for unit tests only.
func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

// buildJWT signs header.payload with the given RSA key and returns the token string.
func buildJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]interface{}) string {
	t.Helper()

	hdr := map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}
	hdrJSON, _ := json.Marshal(hdr)
	hdrB64 := base64.RawURLEncoding.EncodeToString(hdrJSON)

	payJSON, _ := json.Marshal(claims)
	payB64 := base64.RawURLEncoding.EncodeToString(payJSON)

	sigInput := hdrB64 + "." + payB64
	h := crypto.SHA256.New()
	h.Write([]byte(sigInput))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest)
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return sigInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// jwkFromRSAKey returns a JWK JSON document for the public part of key.
func jwkFromRSAKey(key *rsa.PrivateKey, kid string) map[string]interface{} {
	pub := &key.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	// Exponent: convert int to big-endian bytes
	eInt := big.NewInt(int64(pub.E))
	e := base64.RawURLEncoding.EncodeToString(eInt.Bytes())
	return map[string]interface{}{
		"kty": "RSA",
		"kid": kid,
		"alg": "RS256",
		"use": "sig",
		"n":   n,
		"e":   e,
	}
}

// startJWKSServer starts an httptest server that serves the given RSA key as JWKS.
func startJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	jwk := jwkFromRSAKey(key, kid)
	doc := map[string]interface{}{"keys": []interface{}{jwk}}
	body, _ := json.Marshal(doc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const (
	testIssuer   = "https://auth.example.com"
	testAudience = "test-app"
	testKid      = "key-001"
)

func newTestValidator(t *testing.T, key *rsa.PrivateKey) *Validator {
	t.Helper()
	srv := startJWKSServer(t, key, testKid)
	cfg := ValidatorConfig{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  srv.URL,
		JWKSTTL:  time.Minute,
	}
	return NewValidator(cfg)
}

func validClaims() map[string]interface{} {
	return map[string]interface{}{
		"sub": "user-123",
		"iss": testIssuer,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Add(-time.Minute).Unix(),
	}
}

func TestValidator_ValidToken(t *testing.T) {
	key := generateTestRSAKey(t)
	v := newTestValidator(t, key)

	token := buildJWT(t, key, testKid, validClaims())
	claims, err := v.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.Sub != "user-123" {
		t.Errorf("sub = %q, want %q", claims.Sub, "user-123")
	}
}

func TestValidator_ExpiredToken(t *testing.T) {
	key := generateTestRSAKey(t)
	v := newTestValidator(t, key)

	c := validClaims()
	c["exp"] = time.Now().Add(-time.Hour).Unix() // expired
	token := buildJWT(t, key, testKid, c)

	_, err := v.Validate(context.Background(), token)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error message should mention 'expired', got: %v", err)
	}
}

func TestValidator_WrongSignature(t *testing.T) {
	key := generateTestRSAKey(t)
	otherKey := generateTestRSAKey(t) // Different key
	v := newTestValidator(t, key)     // Server only knows 'key'

	// Token signed by otherKey, but server only has 'key' public
	// — we need to serve otherKey JWKS on the server so kid resolves,
	// but the signature won't match.
	// Simpler: tamper with the signature bytes.
	token := buildJWT(t, key, testKid, validClaims())
	parts := strings.Split(token, ".")
	// Corrupt the signature part.
	parts[2] = base64.RawURLEncoding.EncodeToString([]byte("invalidsignature"))
	tampered := strings.Join(parts, ".")

	_, err := v.Validate(context.Background(), tampered)
	if err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
	_ = otherKey // suppress unused variable warning
}

func TestValidator_WrongIssuer(t *testing.T) {
	key := generateTestRSAKey(t)
	v := newTestValidator(t, key)

	c := validClaims()
	c["iss"] = "https://evil.example.com"
	token := buildJWT(t, key, testKid, c)

	_, err := v.Validate(context.Background(), token)
	if err == nil {
		t.Fatal("expected error for wrong issuer, got nil")
	}
	if !strings.Contains(err.Error(), "issuer") {
		t.Errorf("error should mention 'issuer', got: %v", err)
	}
}

func TestValidator_MissingExpClaim(t *testing.T) {
	key := generateTestRSAKey(t)
	v := newTestValidator(t, key)

	c := validClaims()
	delete(c, "exp")
	token := buildJWT(t, key, testKid, c)

	_, err := v.Validate(context.Background(), token)
	if err == nil {
		t.Fatal("expected error for missing exp, got nil")
	}
}

func TestValidator_MalformedToken(t *testing.T) {
	key := generateTestRSAKey(t)
	v := newTestValidator(t, key)

	_, err := v.Validate(context.Background(), "not.a.valid.token.atall")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestValidator_ZitadelRoles(t *testing.T) {
	key := generateTestRSAKey(t)
	v := newTestValidator(t, key)

	c := validClaims()
	c["urn:zitadel:iam:org:project:roles"] = map[string]interface{}{
		"admin": map[string]string{"org-123": "lurus"},
	}
	token := buildJWT(t, key, testKid, c)

	claims, err := v.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.HasAdminRole(claims) {
		t.Error("HasAdminRole should return true for admin role")
	}
}

func TestValidator_NonAdminRoles(t *testing.T) {
	key := generateTestRSAKey(t)
	v := newTestValidator(t, key)

	c := validClaims()
	c["roles"] = []string{"user", "viewer"}
	token := buildJWT(t, key, testKid, c)

	claims, err := v.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.HasAdminRole(claims) {
		t.Error("HasAdminRole should return false for non-admin roles")
	}
}

func TestJWKSCache_Refresh(t *testing.T) {
	key := generateTestRSAKey(t)
	srv := startJWKSServer(t, key, testKid)

	c := NewJWKSCache(srv.URL, time.Minute)
	pub, err := c.GetKey(context.Background(), testKid)
	if err != nil {
		t.Fatalf("GetKey error: %v", err)
	}
	if pub == nil {
		t.Error("expected non-nil public key")
	}
}

func TestJWKSCache_UnknownKid(t *testing.T) {
	key := generateTestRSAKey(t)
	srv := startJWKSServer(t, key, testKid)

	c := NewJWKSCache(srv.URL, time.Minute)
	_, err := c.GetKey(context.Background(), "nonexistent-kid")
	if err == nil {
		t.Fatal("expected error for unknown kid")
	}
}

func TestParseAudience_String(t *testing.T) {
	raw := json.RawMessage(`"my-app"`)
	aud, err := parseAudience(raw)
	if err != nil {
		t.Fatalf("parseAudience error: %v", err)
	}
	if len(aud) != 1 || aud[0] != "my-app" {
		t.Errorf("unexpected aud: %v", aud)
	}
}

func TestParseAudience_Array(t *testing.T) {
	raw := json.RawMessage(`["app1","app2"]`)
	aud, err := parseAudience(raw)
	if err != nil {
		t.Fatalf("parseAudience error: %v", err)
	}
	if fmt.Sprintf("%v", aud) != "[app1 app2]" {
		t.Errorf("unexpected aud: %v", aud)
	}
}
