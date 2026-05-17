package api

import (
	"ai-hub/server/core"
	"ai-hub/server/store"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ScopedFileItem struct {
	FileName        string `json:"file_name"`
	Preview         string `json:"preview"`
	Type            string `json:"type"`
	SourceSessionID int64  `json:"source_session_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	Scope           string `json:"scope"`
	Origin          string `json:"origin"`
	Size            int64  `json:"size"`
}

type scopeEntry struct {
	scope  string
	origin string
}

func resolveTeamScope(sessionID int64, defaultScope string) string {
	if sessionID <= 0 {
		return ""
	}
	sess, err := store.GetSession(sessionID)
	if err != nil {
		return ""
	}
	group := sess.GroupName
	if group == "" {
		return ""
	}
	if defaultScope == "memory" {
		return "memory/" + group
	}
	return group + "/" + defaultScope
}

func resolveSessionScope(sessionID int64, defaultScope string) string {
	if sessionID <= 0 {
		return ""
	}
	if defaultScope == "memory" {
		return "memory/" + strconv.FormatInt(sessionID, 10)
	}
	sess, err := store.GetSession(sessionID)
	if err != nil {
		return ""
	}
	group := sess.GroupName
	if group == "" {
		group = "_standalone"
	}
	return group + "/sessions/" + strconv.FormatInt(sessionID, 10) + "/" + defaultScope
}

func extractScopeGroup(scope string) string {
	parts := strings.Split(scope, "/")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

func isValidScopedScope(scope string) bool {
	if scope == "memory" || scope == "rules" || scope == "global" {
		return true
	}
	if strings.HasPrefix(scope, "memory/") {
		return true
	}
	if parts := strings.Split(scope, "/"); len(parts) == 4 && parts[1] == "sessions" {
		group, idStr, suffix := parts[0], parts[2], parts[3]
		if suffix != "memory" {
			return false
		}
		if !isValidGroupName(group) {
			return false
		}
		if id, err := strconv.ParseInt(idStr, 10, 64); err != nil || id <= 0 {
			return false
		}
		return true
	}

	idx := strings.LastIndex(scope, "/")
	if idx <= 0 {
		return false
	}
	suffix := scope[idx+1:]
	if suffix != "memory" && suffix != "rules" {
		return false
	}
	prefix := scope[:idx]
	if strings.Contains(prefix, "/") {
		return false
	}
	return isValidGroupName(prefix)
}

func scopedFileType(scope string) string {
	parts := strings.Split(scope, "/")
	return parts[len(parts)-1]
}

func scopedFilePreview(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	runes := []rune(string(data))
	if len(runes) > 100 {
		return string(runes[:100])
	}
	return string(runes)
}

func isSupportedScopedFile(name string, typeFilter string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch typeFilter {
	case "memory":
		return ext == ".md" || ext == ".txt"
	case "rules":
		return ext == ".md" || ext == ".txt"
	default:
		return false
	}
}

func scopedFilePriority(item ScopedFileItem, sessionID int64) int {
	if sessionID > 0 && item.SourceSessionID == sessionID {
		return 0
	}
	switch item.Origin {
	case "session":
		return 1
	case "team":
		return 2
	default:
		return 3
	}
}

func ListScopedFiles(c *gin.Context) {
	sessionIDStr := strings.TrimSpace(c.Query("session_id"))
	explicitScope := strings.TrimSpace(c.Query("scope"))
	typeFilter := strings.TrimSpace(c.DefaultQuery("type", "memory"))
	levelFilter := strings.TrimSpace(c.DefaultQuery("level", "all"))

	if typeFilter == "" {
		typeFilter = "memory"
	}
	if typeFilter != "memory" && typeFilter != "rules" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type"})
		return
	}

	var sessionID int64
	if sessionIDStr != "" {
		id, err := strconv.ParseInt(sessionIDStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
			return
		}
		sessionID = id
	}

	var scopes []scopeEntry
	if explicitScope != "" {
		if !isValidScopedScope(explicitScope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scopes = append(scopes, scopeEntry{scope: explicitScope, origin: detectScopeOrigin(explicitScope)})
	} else {
		groupName := ""
		if sessionID > 0 {
			if sess, err := store.GetSession(sessionID); err == nil {
				groupName = sess.GroupName
				if groupName == "" {
					groupName = "_standalone"
				}
			}
		}

		wantSession := levelFilter == "all" || levelFilter == "session"
		wantTeam := levelFilter == "all" || levelFilter == "team"
		wantGlobal := levelFilter == "all" || levelFilter == "global"

		if groupName != "" && wantSession && typeFilter == "memory" {
			scopes = append(scopes, scopeEntry{
				scope:  "memory/" + strconv.FormatInt(sessionID, 10),
				origin: "session",
			})
		}
		if groupName != "" && wantTeam {
			scopeStr := groupName + "/" + typeFilter
			if typeFilter == "memory" {
				scopeStr = "memory/" + groupName
			}
			scopes = append(scopes, scopeEntry{
				scope:  scopeStr,
				origin: "team",
			})
		}
		if wantGlobal || groupName == "" {
			if typeFilter == "memory" {
				scopes = append(scopes, scopeEntry{scope: "global", origin: "global"})
			} else {
				scopes = append(scopes, scopeEntry{scope: typeFilter, origin: "global"})
			}
		}
	}

	items := make([]ScopedFileItem, 0)
	for _, entry := range scopes {
		dir := core.ScopeDir(entry.scope)

		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !isSupportedScopedFile(d.Name(), typeFilter) {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}

			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				relPath = d.Name()
			}
			fileName := filepath.ToSlash(relPath)

			sourceSessionID := int64(0)
			if entry.origin == "session" {
				parts := strings.Split(entry.scope, "/")
				if len(parts) == 4 {
					if sid, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
						sourceSessionID = sid
					}
				} else if len(parts) == 2 && parts[0] == "memory" {
					if sid, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						sourceSessionID = sid
					}
				}
			}
			items = append(items, ScopedFileItem{
				FileName:        fileName,
				Preview:         scopedFilePreview(path),
				Type:            scopedFileType(entry.scope),
				SourceSessionID: sourceSessionID,
				CreatedAt:       fileBirthTime(info).Format(time.RFC3339),
				UpdatedAt:       info.ModTime().Format(time.RFC3339),
				Scope:           entry.scope,
				Origin:          entry.origin,
				Size:            info.Size(),
			})
			return nil
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		pi := scopedFilePriority(items[i], sessionID)
		pj := scopedFilePriority(items[j], sessionID)
		if pi != pj {
			return pi < pj
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})

	c.JSON(http.StatusOK, gin.H{"files": items, "total": len(items)})
}

func detectScopeOrigin(scope string) string {
	if scope == "memory" || scope == "rules" || scope == "global" {
		return "global"
	}
	if strings.HasPrefix(scope, "memory/") {
		suffix := scope[7:]
		if _, err := strconv.ParseInt(suffix, 10, 64); err == nil {
			return "session"
		}
		return "team"
	}
	parts := strings.Split(scope, "/")
	if len(parts) == 4 && parts[1] == "sessions" {
		return "session"
	}
	return "team"
}

func ReadScopedFile(c *gin.Context) {
	var req struct {
		FileName  string `json:"file_name"`
		Scope     string `json:"scope"`
		SessionID int64  `json:"session_id"`
		Type      string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Type == "" {
		req.Type = "memory"
	}
	scope, ok := resolveScopedWriteScope(req.Scope, req.SessionID, req.Type)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	dir := core.ScopeDir(scope)
	path := filepath.Join(dir, req.FileName)
	if !strings.HasPrefix(path, dir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"file_name": req.FileName, "content": string(data), "scope": scope})
}

func WriteScopedFile(c *gin.Context) {
	var req struct {
		FileName  string `json:"file_name"`
		Content   string `json:"content"`
		Scope     string `json:"scope"`
		SessionID int64  `json:"session_id"`
		Type      string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Type == "" {
		req.Type = "memory"
	}
	scope, ok := resolveScopedWriteScope(req.Scope, req.SessionID, req.Type)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if req.Scope != "" && req.SessionID > 0 {
		sessionScope := resolveSessionScope(req.SessionID, req.Type)
		teamScope := resolveTeamScope(req.SessionID, req.Type)
		if req.Scope != sessionScope && req.Scope != teamScope && req.Scope != req.Type {
			c.JSON(http.StatusForbidden, gin.H{"error": "cross-team write denied"})
			return
		}
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	dir := core.ScopeDir(scope)
	path := filepath.Join(dir, req.FileName)
	if !strings.HasPrefix(path, dir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := os.WriteFile(path, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName, "scope": scope})
}

func DeleteScopedFile(c *gin.Context) {
	var req struct {
		FileName  string `json:"file_name"`
		Scope     string `json:"scope"`
		SessionID int64  `json:"session_id"`
		Type      string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Type == "" {
		req.Type = "memory"
	}
	scope, ok := resolveScopedWriteScope(req.Scope, req.SessionID, req.Type)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	dir := core.ScopeDir(scope)
	path := filepath.Join(dir, req.FileName)
	if !strings.HasPrefix(path, dir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName})
}

func resolveScopedWriteScope(scope string, sessionID int64, defaultType string) (string, bool) {
	if defaultType == "" {
		defaultType = "memory"
	}
	if scope != "" {
		if !isValidScopedScope(scope) {
			return "", false
		}
		return scope, true
	}
	if sessionID > 0 && defaultType == "memory" {
		if resolved := resolveSessionScope(sessionID, defaultType); resolved != "" {
			return resolved, true
		}
	}
	if defaultType == "memory" {
		return "global", true
	}
	if defaultType != "rules" {
		return "", false
	}
	return defaultType, true
}
