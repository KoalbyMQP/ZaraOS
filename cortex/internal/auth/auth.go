package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"
)

const (
	codeExpiry   = 120 * time.Second
	replayWindow = 60 * time.Second
)

type PendingPair struct {
	Code      string
	Salt      string
	ExpiresAt time.Time
}

type Session struct {
	Token     string // full derived token, kept in memory for request signing
	Prefix    string // first 6 hex chars, safe to expose for identification
	Label     string
	CreatedAt time.Time
	LastSeen  time.Time
}

type Store struct {
	mu       sync.Mutex
	pending  *PendingPair
	sessions []*Session
}

func NewStore() *Store { return &Store{} }

// StartPair generates a new 6-digit code and random salt.
// The code is returned so the caller can print it on the terminal.
// It is never included in the API response.
func (s *Store) StartPair() (code string, expiresIn int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", 0, fmt.Errorf("rng: %w", err)
	}
	code = fmt.Sprintf("%06d", n.Int64())

	saltB := make([]byte, 32)
	if _, err = rand.Read(saltB); err != nil {
		return "", 0, fmt.Errorf("salt rng: %w", err)
	}

	s.pending = &PendingPair{
		Code:      code,
		Salt:      hex.EncodeToString(saltB),
		ExpiresAt: time.Now().Add(codeExpiry),
	}
	return code, int(codeExpiry.Seconds()), nil
}

// GetPending returns the current pending pair if it exists and hasn't expired.
// Used by the internal bridge endpoint to display the code on the robot's screen.
func (s *Store) GetPending() *PendingPair {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || time.Now().After(s.pending.ExpiresAt) {
		s.pending = nil
		return nil
	}
	cp := *s.pending
	return &cp
}

// CompletePair validates the submitted code and, on success, creates a session.
// Returns the salt so the client can derive the shared token locally.
//
// Token derivation (both sides, never transmitted):
//
//	token = hex(HMAC-SHA256(key=code, message=salt))
func (s *Store) CompletePair(code, label string) (salt string, createdAt time.Time, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pending == nil || time.Now().After(s.pending.ExpiresAt) {
		s.pending = nil
		return "", time.Time{}, fmt.Errorf("invalid or expired code")
	}
	if s.pending.Code != code {
		return "", time.Time{}, fmt.Errorf("invalid or expired code")
	}

	salt = s.pending.Salt
	token := DeriveToken(code, salt)

	if label == "" {
		label = "Web Client"
	}
	now := time.Now()
	s.sessions = append(s.sessions, &Session{
		Token:     token,
		Prefix:    token[:6],
		Label:     label,
		CreatedAt: now,
		LastSeen:  now,
	})
	s.pending = nil
	return salt, now, nil
}

// DeriveToken produces the shared token from the pairing code and salt.
// This is the same computation the client performs after receiving the salt.
func DeriveToken(code, salt string) string {
	mac := hmac.New(sha256.New, []byte(code))
	mac.Write([]byte(salt))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateRequest verifies the timestamp freshness and HMAC-SHA256 request signature.
//
// Signing scheme (client must implement):
//
//	timestamp  = time.Now().UTC().Format(time.RFC3339)
//	bodyHash   = hex(SHA256(raw_request_body))   // "" body → hash of empty string
//	message    = timestamp + "\n" + METHOD + "\n" + path + "\n" + bodyHash
//	signature  = hex(HMAC-SHA256(key=token, message=message))
//
// Headers required on every protected request:
//
//	X-Timestamp: <RFC3339 UTC>
//	X-Signature: <hex signature>
func (s *Store) ValidateRequest(timestamp, method, path, body, sigHex string) (*Session, error) {
	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp format, use RFC3339")
	}
	diff := time.Since(ts)
	if diff < -replayWindow || diff > replayWindow {
		return nil, fmt.Errorf("timestamp outside ±%ds window", int(replayWindow.Seconds()))
	}

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, fmt.Errorf("signature must be hex encoded")
	}

	h := sha256.Sum256([]byte(body))
	bodyHash := hex.EncodeToString(h[:])
	message := timestamp + "\n" + method + "\n" + path + "\n" + bodyHash

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sess := range s.sessions {
		mac := hmac.New(sha256.New, []byte(sess.Token))
		mac.Write([]byte(message))
		if hmac.Equal(mac.Sum(nil), sigBytes) {
			sess.LastSeen = time.Now()
			cp := *sess
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("invalid signature")
}

func (s *Store) ListSessions() []*Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Session, len(s.sessions))
	for i, sess := range s.sessions {
		cp := *sess
		out[i] = &cp
	}
	return out
}

func (s *Store) Revoke(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, sess := range s.sessions {
		if sess.Prefix == prefix {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			return true
		}
	}
	return false
}
