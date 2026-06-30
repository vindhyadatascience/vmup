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

// DetectGCPAccount returns the active gcloud account email
// (e.g. "user@example.com"), or "" if it cannot be determined.
func DetectGCPAccount() string {
	out, err := exec.Command("gcloud", "config", "get-value", "account").Output()
	if err != nil {
		return ""
	}
	acct := strings.TrimSpace(string(out))
	// gcloud prints "(unset)" when no account is configured.
	if acct == "" || strings.EqualFold(acct, "(unset)") {
		return ""
	}
	return acct
}

// DetectUserAccount derives the local part (username) and email domain from the
// active gcloud account. Either value may be empty if detection fails; callers
// fall back to the OS username / an empty domain in that case.
func DetectUserAccount() (username, domain string) {
	acct := DetectGCPAccount()
	if acct == "" {
		return "", ""
	}
	local, dom, ok := strings.Cut(acct, "@")
	if !ok {
		return "", ""
	}
	return local, dom
}

func GeneratePassword() string {
	b := make([]byte, 15)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func GenerateTimestamp() string {
	return time.Now().Format("20060102-150405")
}
