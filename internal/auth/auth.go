package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dominikpalatynski/prout/internal/config"
)

const SessionCookieName = "prout_session"

type Manager struct {
	username      string
	password      string
	sessionTTL    time.Duration
	sessionSecret []byte
}

type Session struct {
	Username  string
	ExpiresAt time.Time
}

type sessionPayload struct {
	Username  string `json:"u"`
	ExpiresAt int64  `json:"exp"`
}

func NewManager(cfg config.AuthConfig) (*Manager, error) {
	if cfg.Username == "" {
		return nil, errors.New("auth username is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("auth password is required")
	}
	if cfg.SessionSecret == "" {
		return nil, errors.New("auth session secret is required")
	}

	sessionTTL, err := time.ParseDuration(cfg.SessionTTL)
	if err != nil {
		return nil, fmt.Errorf("parse auth session ttl: %w", err)
	}
	if sessionTTL <= 0 {
		return nil, errors.New("auth session ttl must be greater than zero")
	}

	return &Manager{
		username:      cfg.Username,
		password:      cfg.Password,
		sessionTTL:    sessionTTL,
		sessionSecret: []byte(cfg.SessionSecret),
	}, nil
}

func (m *Manager) ValidateCredentials(username, password string) bool {
	return constantTimeEqual(username, m.username) && constantTimeEqual(password, m.password)
}

func (m *Manager) NewSessionCookie(secure bool, now time.Time) (*http.Cookie, error) {
	token, expiresAt, err := m.NewSessionToken(now)
	if err != nil {
		return nil, err
	}

	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(m.sessionTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}, nil
}

func ExpiredSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

func (m *Manager) NewSessionToken(now time.Time) (string, time.Time, error) {
	expiresAt := now.UTC().Add(m.sessionTTL)
	payload := sessionPayload{
		Username:  m.username,
		ExpiresAt: expiresAt.Unix(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal session payload: %w", err)
	}

	signature := m.sign(payloadBytes)
	token := base64.RawURLEncoding.EncodeToString(payloadBytes) + "." + base64.RawURLEncoding.EncodeToString(signature)
	return token, expiresAt, nil
}

func (m *Manager) ValidateSessionToken(token string, now time.Time) (*Session, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid session token format")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode session payload: %w", err)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode session signature: %w", err)
	}

	if !hmac.Equal(signature, m.sign(payloadBytes)) {
		return nil, errors.New("invalid session signature")
	}

	var payload sessionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal session payload: %w", err)
	}

	if !constantTimeEqual(payload.Username, m.username) {
		return nil, errors.New("session user does not match configured user")
	}

	expiresAt := time.Unix(payload.ExpiresAt, 0).UTC()
	if !expiresAt.After(now.UTC()) {
		return nil, errors.New("session expired")
	}

	return &Session{
		Username:  payload.Username,
		ExpiresAt: expiresAt,
	}, nil
}

func (m *Manager) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, m.signingKey())
	mac.Write(payload)
	return mac.Sum(nil)
}

func (m *Manager) signingKey() []byte {
	mac := hmac.New(sha256.New, m.sessionSecret)
	mac.Write([]byte(m.username))
	mac.Write([]byte{0})
	mac.Write([]byte(m.password))
	return mac.Sum(nil)
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
