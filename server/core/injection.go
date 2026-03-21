package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Structured memory category definitions.
// Fixed categories are always injected for every new session.
// Conditional categories are injected only when keyword matching succeeds.
var (
	FixedCategories       = []string{"identity", "preferences", "error-genome"}
	ConditionalCategories = []string{"domain", "lessons", "active", "decisions"}
	AllCategories         = append(append([]string{}, FixedCategories...), ConditionalCategories...)
)

// CategoryLabels provides human-readable labels for each category.
var CategoryLabels = map[string]string{
	"identity":    "用户身份画像",
	"preferences": "用户偏好习惯",
	"domain":      "用户领域知识",
	"lessons":     "踩过的坑和教训",
	"error-genome": "AI 常犯错误模式库",
	"active":      "当前进行中的事项",
	"decisions":   "重要决策记录",
}

// InjectionRoute represents a keyword → categories mapping (loaded from store).
type InjectionRoute struct {
	Keywords         string // pipe-separated: "开发|编程|代码"
	InjectCategories string // comma-separated: "domain,lessons"
}

// StructuredMemoryDir returns the directory for structured memory files.
func StructuredMemoryDir() string {
	return filepath.Join(GetDataDir(), "structured-memory")
}

// ReadStructuredMemory reads a single structured memory file by category name.
// Returns empty string if file does not exist.
func ReadStructuredMemory(category string) string {
	path := filepath.Join(StructuredMemoryDir(), category+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteStructuredMemory writes content to a structured memory file.
func WriteStructuredMemory(category, content string) error {
	dir := StructuredMemoryDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, category+".md"), []byte(content), 0644)
}

// MatchInjectionRoutes checks the query against injection routes and returns
// the set of conditional categories that should be injected.
// routes is loaded from the database by the caller.
func MatchInjectionRoutes(query string, routes []InjectionRoute) map[string]bool {
	matched := make(map[string]bool)
	queryLower := strings.ToLower(query)

	for _, route := range routes {
		keywords := strings.Split(route.Keywords, "|")
		for _, kw := range keywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			if strings.Contains(queryLower, strings.ToLower(kw)) {
				// This route matched — add all its categories
				cats := strings.Split(route.InjectCategories, ",")
				for _, cat := range cats {
					cat = strings.TrimSpace(cat)
					if cat != "" {
						matched[cat] = true
					}
				}
				break // One keyword match per route is enough
			}
		}
	}
	return matched
}

// BuildStructuredMemoryBlock builds the structured memory injection block.
// It always includes fixed categories (if they have content),
// and includes conditional categories based on the matchedConditional set.
// Returns empty string if no memory content is available.
func BuildStructuredMemoryBlock(matchedConditional map[string]bool) string {
	var parts []string

	// 1. Fixed categories — always inject
	for _, cat := range FixedCategories {
		content := ReadStructuredMemory(cat)
		if content == "" {
			continue
		}
		label := CategoryLabels[cat]
		parts = append(parts, fmt.Sprintf("## %s (%s)\n\n%s", label, cat, content))
	}

	// 2. Conditional categories — only inject if matched
	for _, cat := range ConditionalCategories {
		if !matchedConditional[cat] {
			continue
		}
		content := ReadStructuredMemory(cat)
		if content == "" {
			continue
		}
		label := CategoryLabels[cat]
		parts = append(parts, fmt.Sprintf("## %s (%s)\n\n%s", label, cat, content))
	}

	if len(parts) == 0 {
		return ""
	}

	// Wrap with structured-memory markers
	return fmt.Sprintf(
		"<!-- structured-memory:start -->\n%s\n<!-- structured-memory:end -->",
		strings.Join(parts, "\n\n"),
	)
}
