// Package tokens defines a token holder to generate and validate access tokens.
//
// The package also provides an in-memory implementation.
//
// Documentation Last Review: 07.10.2020
//
package tokens

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// Holder is a store for access tokens.
type Holder interface {
	// Generate creates a new token that is valid for the provided amount of
	// time.
	Generate(expiration time.Duration) string

	// Verify checks that the given token exists and is still valid.
	Verify(token string) bool
}

// InMemoryHolder stores access token in memory.
//
// - implements tokens.Holder
type InMemoryHolder struct {
	sync.Mutex
	tokens map[string]time.Time
}

// NewInMemoryHolder creates a new empty token holder.
func NewInMemoryHolder() *InMemoryHolder {
	return &InMemoryHolder{
		tokens: make(map[string]time.Time),
	}
}

// Generate implements tokens.Holder. It generates a token that will expire
// after a given amount of time.
func (holder *InMemoryHolder) Generate(expiration time.Duration) string {
	buffer := make([]byte, 16)
	rand.Read(buffer)

	str := base64.StdEncoding.EncodeToString(buffer)

	holder.Lock()
	holder.tokens[str] = time.Now().Add(expiration)
	holder.Unlock()

	return str
}

// Verify implements tokens.Holder. It returns true if the token is valid.
func (holder *InMemoryHolder) Verify(token string) bool {
	holder.Lock()
	defer holder.Unlock()

	deadline, ok := holder.tokens[token]
	if !ok {
		return false
	}

	return deadline.After(time.Now())
}
