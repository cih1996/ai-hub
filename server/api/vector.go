package api

import (
	"ai-hub/server/core"
	"ai-hub/server/store"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// resolveTeamScope looks up a session's group_name and returns the team scope.
// Returns "" if session not found or has no group_name.
func resolveTeamScope(sessionID int64, defaultScope string) string {
	if sessionID <= 0 {
		return ""
	}
	sess, err := store.GetSession(sessionID)
	if err != nil || sess.GroupName == "" {
		return ""
	}
	return sess.GroupName + "/" + defaultScope
}

// resolveSessionScope returns the session-level scope for a given session.
// e.g. "团队名/sessions/21/memory". Returns "" if session has no group_name.
func resolveSessionScope(sessionID int64, defaultScope string) string {
	if sessionID <= 0 {
		return ""
	}
	sess, err := store.GetSession(sessionID)
	if err != nil || sess.GroupName == "" {
		return ""
	}
	return sess.GroupName + "/sessions/" + strconv.FormatInt(sessionID, 10) + "/" + defaultScope
}

// extractScopeGroup extracts the group name from a scope string.
// Returns "" for global scopes ("knowledge", "memory").
func extractScopeGroup(scope string) string {
	parts := strings.Split(scope, "/")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

// waitVectorReady waits for the vector engine to become ready during bootstrap.
// Returns true if ready; returns false and writes 503 response if not.
func waitVectorReady(c *gin.Context) bool {
	if core.Vector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not initialized"})
		return false
	}
	if core.Vector.IsReady() {
		return true
	}
	if core.Vector.IsDisabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine disabled"})
		return false
	}
	// Engine is bootstrapping — wait up to 60s
	log.Printf("[vector-api] engine not ready, waiting for bootstrap (request: %s)", c.Request.URL.Path)
	if core.Vector.WaitReady(60 * time.Second) {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not ready (timeout waiting for bootstrap)"})
	return false
}

// isValidScope returns true if scope is one of the allowed forms:
//   - "knowledge" or "memory" (global)
//   - "<group>/knowledge", "<group>/memory", or "<group>/rules" (team-level)
//   - "<group>/sessions/<id>/knowledge", "<group>/sessions/<id>/memory" (session-level)
//
// groupname supports Unicode letters (including CJK/Chinese), digits, spaces,
// hyphens and underscores. Path traversal sequences are rejected.
func isValidScope(scope string) bool {
	if scope == "knowledge" || scope == "memory" {
		return true
	}
	// Try session-level: <group>/sessions/<id>/<suffix>
	if parts := strings.Split(scope, "/"); len(parts) == 4 && parts[1] == "sessions" {
		group, idStr, suffix := parts[0], parts[2], parts[3]
		if suffix != "knowledge" && suffix != "memory" {
			return false
		}
		if !isValidGroupName(group) {
			return false
		}
		// id must be positive integer
		if id, err := strconv.ParseInt(idStr, 10, 64); err != nil || id <= 0 {
			return false
		}
		return true
	}
	// Team-level: <group>/<suffix>
	idx := strings.LastIndex(scope, "/")
	if idx <= 0 {
		return false
	}
	suffix := scope[idx+1:]
	if suffix != "knowledge" && suffix != "memory" && suffix != "rules" {
		return false
	}
	prefix := scope[:idx]
	if strings.Contains(prefix, "/") {
		return false // unexpected extra slashes
	}
	return isValidGroupName(prefix)
}

// --- Vector MCP tool handlers ---
// These are HTTP endpoints that Claude CLI calls via MCP configuration.

// SearchVector performs semantic search with scope in request body
// POST /api/v1/vector/search
func SearchVector(c *gin.Context) {
	var req struct {
		Scope     string `json:"scope"`
		Query     string `json:"query"`
		TopK      int    `json:"top_k"`
		SessionID int64  `json:"session_id"` // optional: auto-resolve team scope
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scope != "" && !isValidScope(req.Scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be 'knowledge', 'memory', or '<groupname>/knowledge', '<groupname>/memory'"})
		return
	}
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.Scope == "" {
		req.Scope = "knowledge" // default scope for generic search
	}
	if !waitVectorReady(c) {
		return
	}
	results, err := core.Vector.Search(req.Scope, req.Query, req.TopK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// SearchKnowledge performs semantic search on knowledge files
// POST /api/v1/vector/search_knowledge
func SearchKnowledge(c *gin.Context) {
	vectorSearch(c, "knowledge")
}

// SearchMemory performs semantic search on memory files
// POST /api/v1/vector/search_memory
func SearchMemory(c *gin.Context) {
	vectorSearch(c, "memory")
}

// ListKnowledgeFiles lists knowledge files with optional session_id auto-scope.
// GET /api/v1/vector/list_knowledge?session_id=<id>&scope=<optional>
func ListKnowledgeFiles(c *gin.Context) {
	vectorList(c, "knowledge")
}

// ListMemoryFiles lists memory files with optional session_id auto-scope.
// GET /api/v1/vector/list_memory?session_id=<id>&scope=<optional>
func ListMemoryFiles(c *gin.Context) {
	vectorList(c, "memory")
}

// enrichResult adds type and source_session_id to a vector search result.
// origin: "self" | "team" | "global" (used for sorting priority).
func enrichResult(r map[string]interface{}, scopeType, origin string) map[string]interface{} {
	out := make(map[string]interface{}, len(r)+3)
	for k, v := range r {
		out[k] = v
	}
	out["type"] = scopeType // "memory" | "knowledge"
	// Extract source_session_id from metadata (stored as float64 in JSON)
	if meta, ok := r["metadata"].(map[string]interface{}); ok {
		if sid, ok := meta["source_session_id"]; ok {
			out["source_session_id"] = sid
		}
	}
	if _, exists := out["source_session_id"]; !exists {
		out["source_session_id"] = 0
	}
	out["_origin"] = origin // internal sort key, removed before response
	return out
}

// sortPriority returns 0 (self) / 1 (team) / 2 (global) for sorting.
func sortPriority(r map[string]interface{}, sessionID int64) int {
	if sessionID > 0 {
		if sid, ok := r["source_session_id"]; ok {
			var sidInt int64
			switch v := sid.(type) {
			case float64:
				sidInt = int64(v)
			case int64:
				sidInt = v
			case int:
				sidInt = int64(v)
			}
			if sidInt == sessionID {
				return 0 // self
			}
		}
	}
	if origin, ok := r["_origin"].(string); ok && origin == "team" {
		return 1
	}
	return 2 // global
}

func vectorSearch(c *gin.Context, defaultScope string) {
	var req struct {
		Query     string `json:"query"`
		TopK      int    `json:"top_k"`
		Scope     string `json:"scope"`      // optional: explicit scope override
		SessionID int64  `json:"session_id"` // optional: auto-resolve team scope + sorting
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if !waitVectorReady(c) {
		return
	}

	// Determine type label from scope suffix
	scopeType := defaultScope // "memory" or "knowledge"

	// Explicit scope takes priority
	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		// Derive type from explicit scope suffix
		parts := strings.Split(req.Scope, "/")
		scopeType = parts[len(parts)-1]

		results, err := core.Vector.Search(req.Scope, req.Query, req.TopK)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		enriched := make([]map[string]interface{}, 0, len(results))
		for _, r := range results {
			e := enrichResult(r, scopeType, "global")
			delete(e, "_origin")
			enriched = append(enriched, e)
		}
		c.JSON(http.StatusOK, gin.H{"results": enriched})
		return
	}

	// Auto-resolve: if session has group_name, search team scope first then global, merge & sort
	if teamScope := resolveTeamScope(req.SessionID, defaultScope); teamScope != "" {
		var merged []map[string]interface{}
		seen := make(map[string]bool)

		// 1. Search team scope
		teamResults, err := core.Vector.Search(teamScope, req.Query, req.TopK)
		if err != nil {
			log.Printf("[vector] team search error (scope=%s): %v", teamScope, err)
		} else {
			for _, r := range teamResults {
				e := enrichResult(r, scopeType, "team")
				id, _ := e["id"].(string)
				seen[id] = true
				merged = append(merged, e)
			}
		}

		// 2. Search global scope
		globalResults, err2 := core.Vector.Search(defaultScope, req.Query, req.TopK)
		if err2 != nil {
			log.Printf("[vector] global search error (scope=%s): %v", defaultScope, err2)
		} else {
			for _, r := range globalResults {
				id, _ := r["id"].(string)
				if !seen[id] {
					e := enrichResult(r, scopeType, "global")
					merged = append(merged, e)
				}
			}
		}

		// Sort: self(0) > team(1) > global(2), same priority keeps search similarity order
		sortResults(merged, req.SessionID)

		// Remove internal _origin field
		for _, r := range merged {
			delete(r, "_origin")
		}
		c.JSON(http.StatusOK, gin.H{"results": merged})
		return
	}

	// Default: search global scope only
	results, err := core.Vector.Search(defaultScope, req.Query, req.TopK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	enriched := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		e := enrichResult(r, scopeType, "global")
		delete(e, "_origin")
		enriched = append(enriched, e)
	}
	c.JSON(http.StatusOK, gin.H{"results": enriched})
}

// sortResults sorts results in-place: self > team > global, same priority keeps original order.
func sortResults(results []map[string]interface{}, sessionID int64) {
	// Stable sort: assign priority, same priority keeps insertion order (search similarity)
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			pi := sortPriority(results[i], sessionID)
			pj := sortPriority(results[j], sessionID)
			if pj < pi {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// ReadKnowledge reads a knowledge file's full content
// POST /api/v1/vector/read_knowledge
func ReadKnowledge(c *gin.Context) {
	vectorRead(c, "knowledge")
}

// ReadMemory reads a memory file's full content
// POST /api/v1/vector/read_memory
func ReadMemory(c *gin.Context) {
	vectorRead(c, "memory")
}

func vectorRead(c *gin.Context, defaultScope string) {
	var req struct {
		FileName  string `json:"file_name"`
		Scope     string `json:"scope"`      // optional: explicit scope override
		SessionID int64  `json:"session_id"` // optional: auto-resolve team scope
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scope := defaultScope
	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scope = req.Scope
	} else if teamScope := resolveTeamScope(req.SessionID, defaultScope); teamScope != "" {
		scope = teamScope
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

// WriteKnowledge writes/updates a knowledge file
// POST /api/v1/vector/write_knowledge
func WriteKnowledge(c *gin.Context) {
	vectorWrite(c, "knowledge")
}

// WriteMemory writes/updates a memory file
// POST /api/v1/vector/write_memory
func WriteMemory(c *gin.Context) {
	vectorWrite(c, "memory")
}

// WriteVector writes/updates a file in any valid vector scope.
// POST /api/v1/vector/write
func WriteVector(c *gin.Context) {
	// Default scope is only used when request omits scope.
	vectorWrite(c, "knowledge")
}

func vectorWrite(c *gin.Context, defaultScope string) {
	var req struct {
		FileName      string                 `json:"file_name"`
		Content       string                 `json:"content"`
		Scope         string                 `json:"scope"`           // optional: explicit scope override
		SessionID     int64                  `json:"session_id"`      // optional: auto-resolve session/team scope
		ExtraMetadata map[string]interface{} `json:"extra_metadata"`  // optional: structured memory fields (tags/type/status/version etc.)
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Resolve scope with three-layer isolation
	scope := defaultScope
	var sessionGroup string // group of the requesting session
	if req.SessionID > 0 {
		if sess, err := store.GetSession(req.SessionID); err == nil && sess.GroupName != "" {
			sessionGroup = sess.GroupName
		}
	}

	if req.Scope != "" {
		// Explicit scope provided
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		// Cross-team write check: session belongs to groupA but scope targets groupB → 403
		scopeGroup := extractScopeGroup(req.Scope)
		if sessionGroup != "" && scopeGroup != "" && sessionGroup != scopeGroup {
			c.JSON(http.StatusForbidden, gin.H{"error": "cross-team write denied"})
			return
		}
		scope = req.Scope
	} else if sessionGroup != "" {
		// No explicit scope + session has group → default to session-level scope
		scope = sessionGroup + "/sessions/" + strconv.FormatInt(req.SessionID, 10) + "/" + defaultScope
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
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(path, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Trigger vector sync (also registers the dir for watching if new group scope)
	// Pass session_id so it's stored as source_session_id in vector metadata.
	core.SyncFileToVector(scope, path, req.SessionID)

	// If extra_metadata provided, merge into vector record after sync
	if len(req.ExtraMetadata) > 0 && core.Vector != nil {
		docID := filepath.Base(path)
		core.Vector.UpdateMetadata(scope, docID, req.ExtraMetadata)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName, "scope": scope})
}

// DeleteKnowledge deletes a knowledge file
// POST /api/v1/vector/delete_knowledge
func DeleteKnowledge(c *gin.Context) {
	vectorDelete(c, "knowledge")
}

// DeleteMemory deletes a memory file
// POST /api/v1/vector/delete_memory
func DeleteMemory(c *gin.Context) {
	vectorDelete(c, "memory")
}

// DeleteVector deletes a file in any valid vector scope.
// POST /api/v1/vector/delete
func DeleteVector(c *gin.Context) {
	// Default scope is only used when request omits scope.
	vectorDelete(c, "knowledge")
}

func vectorDelete(c *gin.Context, defaultScope string) {
	var req struct {
		FileName  string `json:"file_name"`
		Scope     string `json:"scope"`      // optional: explicit scope override
		SessionID int64  `json:"session_id"` // optional: auto-resolve team scope
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scope := defaultScope
	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scope = req.Scope
	} else if teamScope := resolveTeamScope(req.SessionID, defaultScope); teamScope != "" {
		scope = teamScope
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
	os.Remove(path)
	// Clean vector record
	if core.Vector != nil {
		core.Vector.Delete(scope, req.FileName)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName})
}

// StatsVector returns vector hit statistics
// GET /api/v1/vector/stats?scope=knowledge
func StatsVector(c *gin.Context) {
	scope := c.DefaultQuery("scope", "knowledge")
	if !isValidScope(scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if !waitVectorReady(c) {
		return
	}
	stats, err := core.Vector.Stats(scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// RestartVector restarts the vector engine
// POST /api/v1/vector/restart
func RestartVector(c *gin.Context) {
	if core.Vector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not initialized"})
		return
	}
	go core.Vector.Restart()
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "vector engine restarting"})
}

// VectorStatus returns vector engine status
// GET /api/v1/vector/status
func VectorStatus(c *gin.Context) {
	c.JSON(http.StatusOK, core.Vector.Status())
}

// VectorHealth returns a simplified health check for frontend banner
// GET /api/v1/vector/health
func VectorHealth(c *gin.Context) {
	status := core.Vector.Status()
	ready, _ := status["ready"].(bool)
	disabled, _ := status["disabled"].(bool)
	errMsg, _ := status["error"].(string)

	health := gin.H{
		"ready":    ready,
		"disabled": disabled,
	}
	if errMsg != "" {
		health["error"] = errMsg
	}

	// Check Python availability
	if !ready {
		if _, err := exec.LookPath("python3"); err != nil {
			health["fix_hint"] = "python3_missing"
		} else {
			health["fix_hint"] = "engine_not_ready"
		}
	}

	c.JSON(http.StatusOK, health)
}

// ListVectorFiles lists .md files in a vector scope directory (filesystem only, no engine required).
// GET /api/v1/vector/list?scope=<scope>
// For richer response (type, preview, source_session_id), use GET /api/v1/vector/list_files.
func ListVectorFiles(c *gin.Context) {
	scope := c.Query("scope")
	if scope == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope is required"})
		return
	}
	if !isValidScope(scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	dir := core.ScopeDir(scope)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, []string{})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}
	if files == nil {
		files = []string{}
	}
	c.JSON(http.StatusOK, files)
}

// VectorFileItem represents one file entry in the rich list response.
type VectorFileItem struct {
	FileName        string `json:"file_name"`
	Preview         string `json:"preview"`           // first 100 chars of content
	Type            string `json:"type"`              // "memory" | "knowledge"
	SourceSessionID int64  `json:"source_session_id"` // session that wrote this file (0 if unknown)
	UpdatedAt       string `json:"updated_at"`        // RFC3339 mod time
	Scope           string `json:"scope"`
}

// ListVectorFilesRich lists .md files with preview, type, source_session_id, and updated_at.
// GET /api/v1/vector/list_files?session_id=<id>&scope=<optional>&list_global=<bool>&type=<memory|knowledge|all>
//
// Default behaviour:
//   - Team session:  lists team scope (both knowledge+memory unless type is specified)
//   - No team:       lists files where source_session_id == current session
//   - list_global=true: additionally includes global scope files
//
// Sorting: self (source_session_id == session_id) > team > global; same level by updated_at desc.
func ListVectorFilesRich(c *gin.Context) {
	sessionIDStr := strings.TrimSpace(c.Query("session_id"))
	explicitScope := strings.TrimSpace(c.Query("scope"))
	listGlobal := c.Query("list_global") == "true"
	typeFilter := strings.TrimSpace(c.Query("type")) // "memory" | "knowledge" | "all" | ""

	var sessionID int64
	if sessionIDStr != "" {
		if id, err := strconv.ParseInt(sessionIDStr, 10, 64); err == nil && id > 0 {
			sessionID = id
		}
	}

	type scopeEntry struct {
		scope  string
		origin string // "team" | "global"
	}
	var scopesToList []scopeEntry

	if explicitScope != "" {
		if !isValidScope(explicitScope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scopesToList = append(scopesToList, scopeEntry{scope: explicitScope, origin: "global"})
	} else {
		// Auto-resolve scopes
		types := []string{"memory", "knowledge"}
		if typeFilter == "memory" {
			types = []string{"memory"}
		} else if typeFilter == "knowledge" {
			types = []string{"knowledge"}
		}

		teamScope := ""
		if sessionID > 0 {
			if sess, err := store.GetSession(sessionID); err == nil && sess.GroupName != "" {
				teamScope = sess.GroupName
			}
		}

		if teamScope != "" {
			for _, t := range types {
				scopesToList = append(scopesToList, scopeEntry{scope: teamScope + "/" + t, origin: "team"})
			}
		}
		if listGlobal || teamScope == "" {
			for _, t := range types {
				scopesToList = append(scopesToList, scopeEntry{scope: t, origin: "global"})
			}
		}
	}

	var allItems []VectorFileItem

	for _, se := range scopesToList {
		// Derive type label from scope suffix
		scopeType := se.scope
		if idx := strings.LastIndex(se.scope, "/"); idx >= 0 {
			scopeType = se.scope[idx+1:]
		}

		dir := core.ScopeDir(se.scope)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		// Get vector metadata for source_session_id (best-effort, nil if engine not ready)
		var metaMap map[string]map[string]interface{}
		if core.Vector != nil {
			metaMap = core.Vector.ListMetadata(se.scope)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}

			// Read preview (first 100 runes)
			filePath := filepath.Join(dir, e.Name())
			preview := ""
			if data, readErr := os.ReadFile(filePath); readErr == nil {
				runes := []rune(string(data))
				if len(runes) > 100 {
					preview = string(runes[:100])
				} else {
					preview = string(runes)
				}
			}

			// Extract source_session_id from vector metadata
			var sourceSessionID int64
			if metaMap != nil {
				if meta, ok := metaMap[e.Name()]; ok {
					if sid, ok := meta["source_session_id"]; ok {
						switch v := sid.(type) {
						case float64:
							sourceSessionID = int64(v)
						case int64:
							sourceSessionID = v
						}
					}
				}
			}

			allItems = append(allItems, VectorFileItem{
				FileName:        e.Name(),
				Preview:         preview,
				Type:            scopeType,
				SourceSessionID: sourceSessionID,
				UpdatedAt:       info.ModTime().Format(time.RFC3339),
				Scope:           se.scope,
			})
		}
	}

	// Sort: self > team > global; same level by updated_at desc
	filePriority := func(item VectorFileItem) int {
		if sessionID > 0 && item.SourceSessionID == sessionID {
			return 0
		}
		if strings.Contains(item.Scope, "/") {
			return 1 // team scope
		}
		return 2
	}
	n := len(allItems)
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			pi := filePriority(allItems[i])
			pj := filePriority(allItems[j])
			if pj < pi || (pj == pi && allItems[j].UpdatedAt > allItems[i].UpdatedAt) {
				allItems[i], allItems[j] = allItems[j], allItems[i]
			}
		}
	}

	if allItems == nil {
		allItems = []VectorFileItem{}
	}
	c.JSON(http.StatusOK, gin.H{"files": allItems, "total": len(allItems)})
}

// vectorList lists .md files in defaultScope or resolved team scope.
// Priority: explicit scope > session_id team scope > defaultScope.
func vectorList(c *gin.Context, defaultScope string) {
	scope := strings.TrimSpace(c.Query("scope"))
	sessionIDStr := strings.TrimSpace(c.Query("session_id"))

	if scope != "" {
		if !isValidScope(scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
	} else if sessionIDStr != "" {
		sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
		if err != nil || sessionID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
			return
		}
		if teamScope := resolveTeamScope(sessionID, defaultScope); teamScope != "" {
			scope = teamScope
		} else {
			scope = defaultScope
		}
	} else {
		scope = defaultScope
	}

	dir := core.ScopeDir(scope)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"scope": scope, "files": []string{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}
	if files == nil {
		files = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"scope": scope, "files": files})
}

// ReadVector reads a single file from any valid scope (filesystem only, no engine required).
// POST /api/v1/vector/read
func ReadVector(c *gin.Context) {
	var req struct {
		FileName string `json:"file_name"`
		Scope    string `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isValidScope(req.Scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	dir := core.ScopeDir(req.Scope)
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
	c.JSON(http.StatusOK, gin.H{"file_name": req.FileName, "content": string(data), "scope": req.Scope})
}

// UpdateVectorMetadata merges metadata updates into an existing vector record.
// POST /api/v1/vector/update_metadata
func UpdateVectorMetadata(c *gin.Context) {
	var req struct {
		DocID    string                 `json:"doc_id"`
		Scope    string                 `json:"scope"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isValidScope(req.Scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if req.DocID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "doc_id is required"})
		return
	}
	if req.Metadata == nil || len(req.Metadata) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metadata is required"})
		return
	}
	if !waitVectorReady(c) {
		return
	}
	updated, err := core.Vector.UpdateMetadata(req.Scope, req.DocID, req.Metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "doc_id": req.DocID, "metadata": updated})
}

// GetVectorDoc retrieves a single vector document with its metadata.
// POST /api/v1/vector/get_doc
func GetVectorDoc(c *gin.Context) {
	var req struct {
		DocID string `json:"doc_id"`
		Scope string `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isValidScope(req.Scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if req.DocID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "doc_id is required"})
		return
	}
	if !waitVectorReady(c) {
		return
	}
	doc, err := core.Vector.GetDoc(req.Scope, req.DocID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, doc)
}
