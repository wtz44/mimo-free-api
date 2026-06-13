package convstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// Store manages conversationId → parentId mappings for MiMo conversation reuse.
// MiMo maintains server-side context when the same conversationId + parentId chain is used.
type Store struct {
	mu    sync.RWMutex
	convs map[string]*convState // key: hash of first user message, value: conversationId + parentId
}

type convState struct {
	ConvID   string // random UUID sent to MiMo (unique, no collision with existing MiMo convs)
	ParentID string // last AI response message ID from MiMo SSE
}

func New() *Store {
	return &Store{
		convs: make(map[string]*convState),
	}
}

// DeriveKey generates a stable lookup key from the first user message + model.
// This is used ONLY for local lookup, NOT sent to MiMo.
func DeriveKey(firstUserMsg, model string) string {
	h := sha256.Sum256([]byte(firstUserMsg + "|" + model))
	return fmt.Sprintf("%x", h[:16]) // 32-char hex
}

// GetOrCreate returns the conversationId and parentId for a conversation.
// If no conversation exists for this key, creates a new one with a random UUID.
func (s *Store) GetOrCreate(key string) (convID, parentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cs, ok := s.convs[key]; ok {
		return cs.ConvID, cs.ParentID
	}
	// New conversation: use random UUID to avoid collision with existing MiMo conversations
	newConvID := fmt.Sprintf("%x", [16]byte{}) // placeholder
	// Actually generate a proper UUID
	newConvID = randomHex32()
	s.convs[key] = &convState{ConvID: newConvID, ParentID: "0"}
	return newConvID, "0"
}

// SetParentID updates the last AI response message ID for a conversation.
func (s *Store) SetParentID(key, parentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cs, ok := s.convs[key]; ok {
		cs.ParentID = parentID
	}
}

// randomHex32 generates a random 32-char hex string using crypto/rand.
func randomHex32() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
