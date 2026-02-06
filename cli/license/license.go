package license

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/csv"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	Manager *LicenseManager = DefaultLicenseManager()
	HomeDir string          // Set by config package during initialization
)

const (
	DefaultIssuer = "rh@vonng.com"
	DefaultMode   = "pro"
	DefaultNode   = 0
	issueHistory  = ".pigsty_license.csv"
)

var (
	JwtPattern   = regexp.MustCompile(`^[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+$`)
	JwtClaimList = map[string]string{
		"aud": "audience",
		"sub": "subject",
		"exp": "expiration time",
		"nbf": "not before",
		"iat": "issued at",
		"iss": "issuer",
		"jti": "jwt id",
	}
	// ErrPrivateKeyNotSet indicates private key is not configured
	ErrPrivateKeyNotSet = errors.New("private key not set")
	// ErrInvalidPEMBlock indicates PEM block is invalid
	ErrInvalidPEMBlock = errors.New("invalid PEM block")
	// ErrInvalidPublicKey indicates public key is not ECDSA
	ErrInvalidPublicKey = errors.New("key is not an ECDSA public key")
	// ErrPrivateKeyMismatch indicates private key doesn't match any public key
	ErrPrivateKeyMismatch = errors.New("private key does not match any public key")
)

// LicenseManager manages public/private key pairs and provides license operations
type LicenseManager struct {
	Active     *jwt.Token
	License    string
	Valid      bool
	Hide       bool
	publicKeys []*ecdsa.PublicKey
	privateKey *ecdsa.PrivateKey
}

// InitLicense will init the license manager with viper config
func InitLicense(lic string) {
	if lic == "" {
		lic := viper.GetString("license")
		if lic == "" {
			// logrus.Debugf("no active license configured")
			return
		}
	}
	if err := Manager.Register(lic); err != nil {
		logrus.Debugf("Failed to register license: %v", err)
		return
	}
	if Manager.Active != nil && Manager.Active.Claims != nil {
		claims := Manager.Active.Claims
		aud, _ := claims.GetAudience()
		sub, _ := claims.GetSubject()
		exp, _ := claims.GetExpirationTime()
		logrus.Debugf("License registered: aud = %s, sub = %s, exp = %s", aud, sub, exp)
	}

}

// GetDefaultPublicKey will return a default public key object
func GetDefaultPublicKey() *ecdsa.PublicKey {
	pk := fmt.Sprintf("%s\n%s\n%s\n%s", `-----BEGIN PUBLIC KEY-----`,
		`MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE0YbDCRD9DTKVJiVrQk7ZqHqQGg/U`,
		`RW5R32Hq5FdxtFryFFM0TFOigYSlQmDjn7dyLUJSri+cPeGTTmLhW2ut8A==`,
		`-----END PUBLIC KEY-----`)
	k, _ := LoadECDSAPublicKey(pk)
	return k
}

// NewLicenseManager creates a new LicenseManager with provided public keys
func NewLicenseManager(pubKeys ...string) (*LicenseManager, error) {
	lm := &LicenseManager{publicKeys: []*ecdsa.PublicKey{}}
	lm.publicKeys = append(lm.publicKeys, GetDefaultPublicKey())

	for _, p := range pubKeys {
		if err := lm.AddPublicKey(p); err != nil {
			return nil, fmt.Errorf("failed to load public key from '%s': %w", p, err)
		}
	}
	return lm, nil
}

func DefaultLicenseManager() *LicenseManager {
	lm, _ := NewLicenseManager()
	return lm
}

// SetPrivateKey configures the private key for signing licenses
func (lm *LicenseManager) SetPrivateKey(privateKey string) error {
	key, err := LoadECDSAPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}
	for _, pk := range lm.publicKeys {
		if PublicKeysEqual(pk, &key.PublicKey) {
			lm.privateKey = key
			return nil
		}
	}
	return ErrPrivateKeyMismatch
}

// AddPublicKey adds a new public key if not already present
func (lm *LicenseManager) AddPublicKey(pubKey string) error {
	publicKey, err := LoadECDSAPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("failed to load public key: %w", err)
	}
	for _, pk := range lm.publicKeys {
		if PublicKeysEqual(publicKey, pk) {
			return nil // Already exists
		}
	}
	lm.publicKeys = append(lm.publicKeys, publicKey)
	return nil
}

// IssueLicense issues a new license with full parameter control
func (lm *LicenseManager) IssueLicense(iss, name string, start time.Time, month int, ltype string, node int) (string, error) {
	if lm.privateKey == nil {
		return "", ErrPrivateKeyNotSet
	}

	now := time.Now()
	jti, err := uuid.NewV7AtTime(now)
	if err != nil {
		return "", fmt.Errorf("failed to generate UUID: %w", err)
	}

	exp := start.AddDate(0, month, 0)
	if month == 0 {
		exp = time.Date(2200, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	sub := fmt.Sprintf("type=%s,node=%d", ltype, node)
	nbf := start.AddDate(0, 0, -1)
	claims := jwt.MapClaims{
		"aud": name,
		"iss": iss,
		"sub": sub,
		"iat": now.Unix(),
		"nbf": nbf.Unix(),
		"exp": exp.Unix(),
		"jti": jti.String(),
	}
	lic, err := IssueJWT(lm.privateKey, claims)
	WriteHistory([]string{now.Format(time.DateTime), jti.String(), name, iss, ltype,
		fmt.Sprintf("%d month", month), fmt.Sprintf("%d node", node),
		nbf.Format(time.DateOnly), exp.Format(time.DateOnly), lic}...)
	return lic, err
}

// Validate JWT from all public keys
func (lm *LicenseManager) Validate(license string) (t *jwt.Token, err error) {
	for _, pk := range lm.publicKeys {
		t, err = jwt.Parse(license, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodES256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return pk, nil
		})
		if err == nil && t.Valid {
			return
		}
	}
	return nil, fmt.Errorf("failed to validate token: %w", err)
}

func (lm *LicenseManager) DescribeDefault() {
	if lm.License != "" {
		lm.Describe(Manager.License)
	}
}

// Describe will print license information
func (lm *LicenseManager) Describe(tokenString string) {
	t, err := lm.Validate(tokenString)
	if err != nil {
		logrus.Debugf("invalid token: %v", err)
	}

	// Try to parse even if validation failed
	rawToken, parseErr := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return nil, nil // Skip signature verification
	})

	if rawToken == nil {
		logrus.Errorf("failed to parse token: %v", parseErr)
		return
	}

	claims, ok := rawToken.Claims.(jwt.MapClaims)
	if !ok {
		logrus.Error("failed to parse claims")
		return
	}
	if !lm.Hide {
		fmt.Printf("raw : %s\n", tokenString)
	}
	sig := "invalid"
	if t != nil && t.Valid {
		sig = "valid"
	}
	fmt.Printf("sig : %-36s %s\n", sig, "(Signature)")
	if aud, found := claims["aud"]; found {
		fmt.Printf("aud : %-36s %s\n", aud, "(Audience)")
	}
	if sub, err := claims.GetSubject(); err == nil {
		fmt.Printf("sub : %-36s %s\n", sub, "(Subject)")
	}
	if exp, err := claims.GetExpirationTime(); err == nil {
		fmt.Printf("exp : %-36s %s\n", exp, "(Expire)")
	}
	if nbf, err := claims.GetNotBefore(); err == nil {
		fmt.Printf("nbf : %-36s %s\n", nbf, "(Not Before)")
	}
	if iat, err := claims.GetIssuedAt(); err == nil {
		fmt.Printf("iat : %-36s %s\n", iat, "(Issued At)")
	}
	if iss, err := claims.GetIssuer(); err == nil {
		fmt.Printf("iss : %-36s %s\n", iss, "(Issuer)")
	}
	if jti, ok := claims["jti"]; ok {
		fmt.Printf("jti : %-36s %s\n", jti, "(Token ID)")
	}
	// iterate other claims
	for k, v := range claims {
		if _, found := JwtClaimList[k]; !found {
			fmt.Printf("    %s: %v\n", k, v)
		}
	}
}

// Register validates and sets the active license
func (lm *LicenseManager) Register(license string) error {
	lm.License = license
	token, err := lm.Validate(license)
	if err != nil {
		return err
	}
	if token != nil {
		lm.Active = token
		if token.Valid {
			lm.Valid = true
		}
	}
	return nil
}

// LicenseType returns the license type from claim subject.
func (lm *LicenseManager) LicenseType() string {
	if !lm.Valid {
		return ""
	}
	sub, err := lm.Active.Claims.GetSubject()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`type=(\w+)`)
	matches := re.FindStringSubmatch(sub)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// WriteHistory writes a license history record to a CSV file, it's ok to skip error
func WriteHistory(record ...string) {
	home := HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	historyFile := filepath.Join(home, issueHistory)
	f, err := os.OpenFile(historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logrus.Errorf("failed to open history file: %v", err)
		return
	}
	defer f.Close()

	// Write CSV record
	writer := csv.NewWriter(f)
	defer writer.Flush()
	if err := writer.Write(record); err != nil {
		logrus.Errorf("failed to write history: %v", err)
	} else {
		logrus.Debugf("wrote license history %s: %s", historyFile, strings.Join(record, ","))
	}
}

// ReadHistory will tabulate the license issue history
func ReadHistory() {
	home := HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	historyFile := filepath.Join(home, issueHistory)

	f, err := os.Open(historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		logrus.Errorf("failed to open history file: %v", err)
		return
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		logrus.Errorf("failed to read history: %v", err)
		return
	}

	if len(records) == 0 {
		logrus.Errorf("No license history found")
		return
	}

	// Find the maximum width for each column
	colWidths := make([]int, len(records[0]))
	for _, record := range records {
		for i, field := range record {
			if len(field) > colWidths[i] {
				colWidths[i] = len(field)
			}
		}
	}

	// Print records with proper spacing
	for _, record := range records {
		for i, field := range record {
			fmt.Printf("%-*s", colWidths[i]+2, field)
		}
		fmt.Println()
	}
}

// GetLicense validates JWT or loads from path then validates
func GetLicense(tokenOrPath string) (string, error) {
	if !IsValidJWT(tokenOrPath) && IsValidPath(tokenOrPath) {
		data, err := os.ReadFile(tokenOrPath)
		if err != nil {
			return "", fmt.Errorf("failed to read license file: %w", err)
		}
		tokenOrPath = strings.TrimRight(string(data), "\n")
		if !IsValidJWT(tokenOrPath) {
			return "", errors.New("invalid token format")
		}
	}
	return tokenOrPath, nil
}

// IsValidJWT checks if a string matches JWT format
func IsValidJWT(token string) bool {
	return JwtPattern.MatchString(token)
}

// IsValidPath checks if a path exists
func IsValidPath(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IssueJWT signs a JWT token with the given private key
func IssueJWT(privateKey *ecdsa.PrivateKey, claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	return token.SignedString(privateKey)
}

// LoadECDSAPrivateKey loads a private key from string or file
func LoadECDSAPrivateKey(pathOrStr string) (*ecdsa.PrivateKey, error) {
	block, err := loadPEMBlock(pathOrStr)
	if err != nil {
		return nil, err
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	return privateKey, nil
}

// LoadECDSAPublicKey loads a public key from string or file
func LoadECDSAPublicKey(pathOrStr string) (*ecdsa.PublicKey, error) {
	block, err := loadPEMBlock(pathOrStr)
	if err != nil {
		return nil, err
	}

	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("invalid PEM block type: %s, expected: PUBLIC KEY", block.Type)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, ErrInvalidPublicKey
	}

	return ecdsaPub, nil
}

// loadPEMBlock loads a PEM block from a string or file
func loadPEMBlock(pathOrStr string) (*pem.Block, error) {
	var data []byte
	if strings.Contains(pathOrStr, "-----BEGIN") && strings.Contains(pathOrStr, "-----END") {
		data = []byte(pathOrStr)
	} else {
		var err error
		data, err = os.ReadFile(pathOrStr)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrInvalidPEMBlock
	}
	return block, nil
}

// PublicKeysEqual compares two ECDSA public keys for equality
func PublicKeysEqual(k1, k2 *ecdsa.PublicKey) bool {
	return k1.X.Cmp(k2.X) == 0 &&
		k1.Y.Cmp(k2.Y) == 0 &&
		k1.Curve.Params().Name == k2.Curve.Params().Name
}

func AddLicense(license string) error {
	viper.Set("license", license)
	return viper.WriteConfig()
}
