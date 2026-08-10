package delivery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

func Sign(secret string, body []byte, ts time.Time) (timestamp string, signature string) {
	timestamp = strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	signature = hex.EncodeToString(mac.Sum(nil))
	return timestamp, signature
}

func SignatureHeader(signature string) string {
	return fmt.Sprintf("sha256=%s", signature)
}
