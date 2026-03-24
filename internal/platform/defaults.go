package platform

import (
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

func DetectUsername() string {
	u, err := user.Current()
	if err != nil {
		return "user"
	}
	return u.Username
}

func DetectGCPProject() string {
	out, err := exec.Command("gcloud", "config", "get-value", "project").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func GeneratePassword() string {
	b := make([]byte, 15)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func GenerateTimestamp() string {
	return time.Now().Format("20060102-150405")
}
