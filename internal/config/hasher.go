package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// HashConfigFile reads the config file, computes its SHA-256 hash,
// and returns the first 16 hex characters.
func HashConfigFile(path string) (string, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read file: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])[:16], data, nil
}
