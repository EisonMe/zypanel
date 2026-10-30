// Package auth implements password verification, signed sessions and CSRF tokens.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Manager holds the single admin credential and the session signing key.
type Manager struct {
	user   string
	hash   []byte
	secret []byte
	ttl    time.Duration
}

// NewManager builds a manager. A bcrypt hash takes precedence over a plain password.
func NewManager(user, plain, hash, secret string, ttl time.Duration) (*Manager, error) {
	if user == "" {
		return nil, errors.New("admin user must not be empty")
	}
	if len(secret) < 16 {
		return nil, errors.New("session secret must be at least 16 bytes")
	}

	var h []byte
	if hash != "" {
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return nil, errors.New("ZYPANEL_ADMIN_PASSWORD_HASH is not a valid bcrypt hash")
		}
		h = []byte(hash)
	} else {
		if plain == "" {
			return nil, errors.New("no admin password configured")
		}
		generated, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		h = generated
	}

	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Manager{user: user, hash: h, secret: []byte(secret), ttl: ttl}, nil
}

// Authenticate checks a username/password pair in constant time where possible.
func (m *Manager) Authenticate(username, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(m.user)) == 1
	passOK := bcrypt.CompareHashAndPassword(m.hash, []byte(password)) == nil
	return userOK && passOK
}

// NewSession returns a signed token of the form base64(payload).base64(sig).
func (m *Manager) NewSession() string {
	exp := time.Now().Add(m.ttl).Unix()
	payload := m.user + "|" + strconv.FormatInt(exp, 10)
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return body + "." + base64.RawURLEncoding.EncodeToString(m.sign(body))
}

func (m *Manager) sign(body string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(body))
	return mac.Sum(nil)
}

// Validate verifies a session token and returns the username when still valid.
func (m *Manager) Validate(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	expected := m.sign(parts[0])
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, got) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	fields := strings.SplitN(string(raw), "|", 2)
	if len(fields) != 2 {
		return "", false
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return fields[0], true
}

// TTL exposes the configured session lifetime.
func (m *Manager) TTL() time.Duration { return m.ttl }

// NewCSRFToken returns a random token for the double-submit cookie pattern.
func NewCSRFToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// CSRFValid compares the cookie token with the submitted form value.
func CSRFValid(cookieValue, formValue string) bool {
	if cookieValue == "" || formValue == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieValue), []byte(formValue)) == 1
}
