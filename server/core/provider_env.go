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

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func mergeNoProxy(current string) string {
	seen := map[string]bool{}
	parts := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		parts = append(parts, v)
	}
	for _, p := range strings.Split(current, ",") {
		add(p)
	}
	add("localhost")
	add("127.0.0.1")
	return strings.Join(parts, ",")
}

func getEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

// applyProviderProxyEnv applies per-provider proxy override to claude subprocess env.
// When proxyURL is empty, it preserves inherited proxy values but still ensures localhost bypass.
func applyProviderProxyEnv(env []string, proxyURL string) []string {
	if strings.TrimSpace(proxyURL) != "" {
		env = upsertEnv(env, "HTTP_PROXY", proxyURL)
		env = upsertEnv(env, "HTTPS_PROXY", proxyURL)
		env = upsertEnv(env, "ALL_PROXY", proxyURL)
		env = upsertEnv(env, "http_proxy", proxyURL)
		env = upsertEnv(env, "https_proxy", proxyURL)
		env = upsertEnv(env, "all_proxy", proxyURL)
	}

	noProxy := getEnvValue(env, "NO_PROXY")
	if noProxy == "" {
		noProxy = getEnvValue(env, "no_proxy")
	}
	merged := mergeNoProxy(noProxy)
	env = upsertEnv(env, "NO_PROXY", merged)
	env = upsertEnv(env, "no_proxy", merged)
	return env
}
