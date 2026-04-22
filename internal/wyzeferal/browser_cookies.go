package wyzeferal

import (
	"fmt"
	"os/exec"
	"strings"
)

const browserHarnessScript = `
cookies = cdp("Network.getCookies", urls=["https://services.wyze.com"])
session = next((c["value"] for c in cookies["cookies"] if c["name"] == "session"), None)
remember = next((c["value"] for c in cookies["cookies"] if c["name"] == "remember_token"), None)
print("SESSION:" + (session or "NONE"))
print("REMEMBER:" + (remember or "NONE"))
`

// extractBrowserCookies shells out to browser-harness to extract fresh
// services.wyze.com session and remember_token cookies from the running Chrome
// instance. This is the self-heal path when the session cookie expires.
func extractBrowserCookies() (session, remember string, err error) {
	cmd := exec.Command("browser-harness")
	cmd.Stdin = strings.NewReader(browserHarnessScript)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("browser-harness: %w", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "SESSION:") {
			session = strings.TrimPrefix(line, "SESSION:")
		}
		if strings.HasPrefix(line, "REMEMBER:") {
			remember = strings.TrimPrefix(line, "REMEMBER:")
		}
	}
	if session == "" || session == "NONE" {
		return "", "", fmt.Errorf("browser-harness returned no session cookie — is Chrome running and logged into my.wyze.com?")
	}
	return session, remember, nil
}
