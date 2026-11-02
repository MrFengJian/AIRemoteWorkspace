package appdeploy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// randomHex 返回 n 字节的十六进制随机串（任务/记录 ID 用）。
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败极罕见；退化为纳秒时间戳保证可用性
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
