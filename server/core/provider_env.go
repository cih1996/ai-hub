package core

import (
	"net/url"
	"strings"
)

func isOllamaBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()

	if strings.Contains(host, "ollama") {
		return true
	}
	return (host == "localhost" || host == "127.0.0.1") && port == "11434"
}

func filterAnthropicEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") ||
			strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN=") ||
			strings.HasPrefix(e, "ANTHROPIC_BASE_URL=") {
			continue
		}
		out = append(out, e)
	}
	return out
}
