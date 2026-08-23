package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const envelopeVersion byte = 1

type Sealer struct {
	aead cipher.AEAD
}

func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

func (s *Sealer) Seal(plaintext []byte, context string) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("create nonce: %w", err)
	}
	result := make([]byte, 1, 1+len(nonce)+len(plaintext)+s.aead.Overhead())
	result[0] = envelopeVersion
	result = append(result, nonce...)
	result = s.aead.Seal(result, nonce, plaintext, []byte(context))
	return result, nil
}

func (s *Sealer) Open(envelope []byte, context string) ([]byte, error) {
	if len(envelope) < 1+s.aead.NonceSize()+s.aead.Overhead() || envelope[0] != envelopeVersion {
		return nil, errors.New("invalid encrypted envelope")
	}
	nonce := envelope[1 : 1+s.aead.NonceSize()]
	ciphertext := envelope[1+s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(context))
	if err != nil {
		return nil, errors.New("encrypted value cannot be opened")
	}
	return plaintext, nil
}

func RandomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, value)
	return value, err
}

func RandomToken(size int) (string, error) {
	raw, err := RandomBytes(size)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func Fingerprint(value []byte) string {
	hash := sha256.Sum256(value)
	return base64.RawURLEncoding.EncodeToString(hash[:9])
}

func SHA256(value string) []byte {
	hash := sha256.Sum256([]byte(value))
	return hash[:]
}
