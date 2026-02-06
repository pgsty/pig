package license

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	defaultPublicKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE0YbDCRD9DTKVJiVrQk7ZqHqQGg/U
RW5R32Hq5FdxtFryFFM0TFOigYSlQmDjn7dyLUJSri+cPeGTTmLhW2ut8A==
-----END PUBLIC KEY-----`
	testPublicKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEAJoxqViuKuTNF4e+Swn+XS+Jsgu9
pWHGOGnkpl4F8gnze+r3Z2o75nK5WMMtcAkhtj3D7dPMD2L9TBUXYs5Srg==
-----END PUBLIC KEY-----`
	testPrivateKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIg0iYoLXTJZUa1UJyo8ugSZbNmwEbuv3Gcr83TgDwq4oAoGCCqGSM49
AwEHoUQDQgAEAJoxqViuKuTNF4e+Swn+XS+Jsgu9pWHGOGnkpl4F8gnze+r3Z2o7
5nK5WMMtcAkhtj3D7dPMD2L9TBUXYs5Srg==
-----END EC PRIVATE KEY-----`
)

// Test loading default public key
func TestGetDefaultPublicKey(t *testing.T) {
	pub := GetDefaultPublicKey()
	if pub == nil {
		t.Fatal("default public key should not be nil")
	}
}

// Test loading public and private keys
func TestLoadKeys(t *testing.T) {
	// Test loading public key from string
	pubKey, err := LoadECDSAPublicKey(testPublicKey)
	if err != nil {
		t.Fatalf("failed to load public key from string: %v", err)
	}
	if pubKey == nil {
		t.Fatal("loaded public key should not be nil")
	}

	// Test loading private key from string
	privKey, err := LoadECDSAPrivateKey(testPrivateKey)
	if err != nil {
		t.Fatalf("failed to load private key from string: %v", err)
	}
	if privKey == nil {
		t.Fatal("loaded private key should not be nil")
	}

	// Test loading from invalid strings
	_, err = LoadECDSAPublicKey("invalid key")
	if err == nil {
		t.Fatal("should fail when loading invalid public key")
	}

	_, err = LoadECDSAPrivateKey("invalid key")
	if err == nil {
		t.Fatal("should fail when loading invalid private key")
	}

	// Test loading from non-existent file
	_, err = LoadECDSAPublicKey("/nonexistent/path")
	if err == nil {
		t.Fatal("should fail when loading public key from non-existent file")
	}

	_, err = LoadECDSAPrivateKey("/nonexistent/path")
	if err == nil {
		t.Fatal("should fail when loading private key from non-existent file")
	}
}

// Test creating LicenseManager
func TestNewLicenseManager(t *testing.T) {
	lm, err := NewLicenseManager()
	if err != nil {
		t.Fatalf("failed to create LicenseManager: %v", err)
	}
	if len(lm.publicKeys) == 0 {
		t.Fatal("LicenseManager should have at least one public key (default)")
	}
}

// Test adding additional public key (using default public key itself for testing)
func TestAddPublicKey(t *testing.T) {
	lm, err := NewLicenseManager()
	if err != nil {
		t.Fatal(err)
	}
	err = lm.AddPublicKey(defaultPublicKey)
	if err != nil {
		t.Fatalf("failed to add public key: %v", err)
	}
	if len(lm.publicKeys) != 1 {
		t.Fatalf("public key count should remain 1 after adding the same key")
	}

	err = lm.AddPublicKey(testPublicKey)
	if err != nil {
		t.Fatalf("failed to add public key: %v", err)
	}
	if len(lm.publicKeys) != 2 {
		t.Fatalf("public key count should be 2 after adding the another public key")
	}

}

// Test loading private key (need to provide matching public-private key pair for testing)
func TestSetPrivateKey(t *testing.T) {
	lm, err := NewLicenseManager()
	if err != nil {
		t.Fatal(err)
	}
	if err := lm.AddPublicKey(testPublicKey); err != nil {
		t.Fatalf("failed to add test public key: %v", err)
	}

	// Should fail when trying to issue without setting private key
	_, err = lm.IssueLicenseFast("test")
	if err == nil {
		t.Fatal("should fail to issue license without private key")
	}

	// Set private key using the matching embedded test key
	err = lm.SetPrivateKey(testPrivateKey)
	if err != nil {
		t.Fatalf("failed to set private key: %v", err)
	}

	// Should succeed when issuing again
	token, err := lm.IssueLicenseFast("test-user")
	if err != nil {
		t.Fatalf("failed to issue license after setting private key: %v", err)
	}
	if !IsValidJWT(token) {
		t.Fatalf("issued token is not valid JWT: %s", token)
	}
}

// Test IssueLicense (with expiration time)
func TestIssueLicense(t *testing.T) {
	lm, _ := NewLicenseManager()
	lm.AddPublicKey(testPublicKey)
	if err := lm.SetPrivateKey(testPrivateKey); err != nil {
		t.Fatal(err)
	}

	start := time.Now().Add(-24 * time.Hour)
	tokenStr, err := lm.IssueLicense("issuer", "test-user", start, 1, "pro", 2)
	if err != nil {
		t.Fatalf("failed to issue license: %v", err)
	}

	if !IsValidJWT(tokenStr) {
		t.Fatalf("issued token is not valid JWT: %s", tokenStr)
	}

	// Verify issued JWT
	tok, err := lm.Validate(tokenStr)
	if err != nil {
		t.Fatalf("failed to validate issued token: %v", err)
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims is not MapClaims")
	}

	if claims["aud"] != "test-user" {
		t.Fatalf("unexpected aud claim, got: %v", claims["aud"])
	}
}

// Test Validate
func TestValidate(t *testing.T) {
	lm, _ := NewLicenseManager()
	lm.AddPublicKey(testPublicKey)
	lm.SetPrivateKey(testPrivateKey)
	tokenStr, err := lm.IssueLicenseFast("test-user")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := lm.Validate(tokenStr)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}
	if !tok.Valid {
		t.Fatal("token should be valid")
	}
}

// Test IsValidJWT
func TestIsValidJWT(t *testing.T) {
	valid := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJ0ZXN0In0.ZTJtOTQ5M3Nz"
	if !IsValidJWT(valid) {
		t.Fatal("valid JWT pattern should return true")
	}

	invalid := "this.is.not.jwt"
	if IsValidJWT(invalid) {
		t.Fatal("invalid JWT pattern should return false")
	}
}

// Test IssueJWT and ValidateJWT
func TestIssueAndValidateJWT(t *testing.T) {
	// Prepare public-private keys (must match)
	privateKeyPem := testPrivateKey
	publicKeyPem := testPublicKey

	privKey, err := LoadECDSAPrivateKey(privateKeyPem)
	if err != nil {
		t.Fatalf("failed to load private key: %v", err)
	}
	claims := jwt.MapClaims{
		"aud": "test-aud",
		"iss": "test-iss",
		"sub": "test-sub",
	}
	tokenStr, err := IssueJWT(privKey, claims)
	if err != nil {
		t.Fatalf("failed to issue JWT: %v", err)
	}

	pubKey, err := LoadECDSAPublicKey(publicKeyPem)
	if err != nil {
		t.Fatalf("failed to load public key: %v", err)
	}

	tok, err := ValidateJWT(tokenStr, pubKey)
	if err != nil {
		t.Fatalf("failed to validate JWT: %v", err)
	}

	if !tok.Valid {
		t.Fatal("issued token should be valid")
	}
}

// Test PublicKeysEqual
func TestPublicKeysEqual(t *testing.T) {
	pub1 := GetDefaultPublicKey()
	pub2 := GetDefaultPublicKey()

	if !PublicKeysEqual(pub1, pub2) {
		t.Fatalf("the same public keys should be equal")
	}
}

// TestDescribe tests license description functionality
func TestDescribe(t *testing.T) {
	lm := &LicenseManager{}
	if err := lm.AddPublicKey(testPublicKey); err != nil {
		t.Fatalf("failed to add public key: %v", err)
	}

	// Test valid JWT
	privKey, err := LoadECDSAPrivateKey(testPrivateKey)
	if err != nil {
		t.Fatalf("failed to load private key: %v", err)
	}

	validClaims := jwt.MapClaims{
		"aud": "test-user",
		"iss": "test-issuer",
		"sub": "type=pro,node=1",
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"nbf": time.Now().Add(-1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"jti": "test-id",
	}
	validToken, err := IssueJWT(privKey, validClaims)
	if err != nil {
		t.Fatalf("failed to issue valid JWT: %v", err)
	}

	// Test expired JWT
	expiredClaims := jwt.MapClaims{
		"aud": "test-user",
		"exp": time.Now().Add(-24 * time.Hour).Unix(), // Expired
	}
	expiredToken, _ := IssueJWT(privKey, expiredClaims)

	// Test malformed JWT
	malformedToken := "invalid.jwt.token"

	// Test unsupported signing method
	unsupportedToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	tests := []struct {
		name  string
		token string
	}{
		{"Valid JWT", validToken},
		{"Expired JWT", expiredToken},
		{"Malformed JWT", malformedToken},
		{"Unsupported Algorithm", unsupportedToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily assert the output since it goes to logrus
			// But we can verify it doesn't panic
			lm.Describe(tt.token)
		})
	}
}
