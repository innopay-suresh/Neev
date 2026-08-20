package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"
)

// coturn recomputes the HMAC itself from the username, so the format is a
// contract with another process: "<unix-expiry>:<id>", password =
// base64(HMAC-SHA1(username, secret)). Get either wrong and every relayed
// session fails authentication — with no error the user can act on.
func TestTURNCredentialsMatchCoturnScheme(t *testing.T) {
	const secret = "s3cr3t"
	user, pass := turnCredentials(secret, "neev", time.Hour)

	parts := strings.SplitN(user, ":", 2)
	if len(parts) != 2 || parts[1] != "neev" {
		t.Fatalf("username %q must be <expiry>:<id>", user)
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		t.Fatalf("expiry is not a unix timestamp: %v", err)
	}
	if exp <= time.Now().Unix() {
		t.Error("credential is already expired on issue")
	}

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(user))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); pass != want {
		t.Errorf("password does not match coturn's HMAC: got %q want %q", pass, want)
	}
}

// The point of the change is that a leaked credential stops working. A
// credential that never expires is the bug this replaces.
func TestTURNCredentialsExpire(t *testing.T) {
	user, _ := turnCredentials("s", "neev", time.Minute)
	exp, _ := strconv.ParseInt(strings.SplitN(user, ":", 2)[0], 10, 64)
	if exp > time.Now().Add(2*time.Minute).Unix() {
		t.Error("TTL not honoured — credential outlives its window")
	}
}
