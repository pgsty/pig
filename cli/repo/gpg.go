package repo

import (
	"bytes"
	"fmt"
	"io"

	_ "embed"

	"golang.org/x/crypto/openpgp/armor"
)

//go:embed assets/key.gpg
var embedGPGKey []byte

// AddDebGPGKey adds the Pigsty GPG key to the Debian repository
func AddDebGPGKey() error {
	block, _ := armor.Decode(bytes.NewReader(embedGPGKey))
	keyBytes, err := io.ReadAll(block.Body)
	if err != nil {
		return fmt.Errorf("failed to read GPG key: %v", err)
	}
	return TryReadMkdirWrite(pigstyDebGPGPath, keyBytes)
}

// AddRpmGPGKey adds the Pigsty GPG key to the RPM repository
func AddRpmGPGKey() error {
	return TryReadMkdirWrite(pigstyRpmGPGPath, embedGPGKey)
}
