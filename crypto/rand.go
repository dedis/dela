//
// Documentation Last Review: 05.10.2020
//

package crypto

import "crypto/rand"

// CryptographicRandomGenerator is cryptographically secure random generator.
//
// - implements crypto.RandGenerator
type CryptographicRandomGenerator struct{}

// Read implements crypto.RandGenerator. It fills the given buffer at its
// capacity as long as no error occurred.
func (crg CryptographicRandomGenerator) Read(buffer []byte) (int, error) {
	return rand.Read(buffer)
}
