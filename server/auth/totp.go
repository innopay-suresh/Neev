package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func base32NoPadEncode(data []byte) string {
	return totpEncoding.EncodeToString(data)
}

func base32NoPadDecode(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.TrimSpace(secret))
	return totpEncoding.DecodeString(normalized)
}

func totpCounter(t time.Time, step time.Duration) uint64 {
	return uint64(t.UTC().UnixNano() / step.Nanoseconds())
}

func totpCode(secret string, counter uint64, digits int) (string, error) {
	key, err := base32NoPadDecode(secret)
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binaryCode := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)
	mod := int(math.Pow10(digits))
	value := binaryCode % mod
	return fmt.Sprintf("%0*d", digits, value), nil
}

// VerifyTOTP checks a 6-digit time-based one-time password with a ±1 step window.
func VerifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) < 6 {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}
	step := 30 * time.Second
	counter := totpCounter(now, step)
	for _, offset := range []int64{-1, 0, 1} {
		expected, err := totpCode(secret, uint64(int64(counter)+offset), 6)
		if err == nil && hmac.Equal([]byte(code), []byte(expected)) {
			return true
		}
	}
	return false
}
