package store

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// newID 生成一个短小且足够唯一的记录 ID。
func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + time.Now().Format("060102")
}
