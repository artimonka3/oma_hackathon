// Package id генерирует короткие уникальные идентификаторы вида prefix_ab12cd34.
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// New возвращает идентификатор вида prefix_ab12cd34.
func New(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
