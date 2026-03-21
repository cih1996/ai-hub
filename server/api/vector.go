package api

import (
	"ai-hub/server/core"
	"ai-hub/server/store"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// resolveTeamScope looks up a session's group_name and returns the team scope.
// Returns "" if session not found. Uses "_standalone" when group_name is empty.
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
		group = "_standalone"
	}
	return group + "/" + defaultScope
}

// resolveSessionScope returns the session-level scope for a given session.
// e.g. "团队名/sessions/21/memory". Uses "_standalone" when group_name is empty.
func resolveSessionScope(sessionID int64, defaultScope string) string {
	if sessionID <= 0 {
		return ""
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

// extractScopeGroup extracts the group name from a scope string.
// Returns "" for global scopes ("memory").
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
//   - "memory" (global)
//   - "memory" (global)
//   - "<group>/memory" or "<group>/rules" (team-level)
//   - "<group>/sessions/<id>/memory" (session-level)
//
// groupname supports Unicode letters (including CJK/Chinese), digits, spaces,
// hyphens and underscores. Path traversal sequences are rejected.
func isValidScope(scope string) bool {
	if scope == "memory" {
		return true
	}
	// Try session-level: <group>/sessions/<id>/<suffix>
	if parts := strings.Split(scope, "/"); len(parts) == 4 && parts[1] == "sessions" {
		group, idStr, suffix := parts[0], parts[2], parts[3]
		if suffix != "memory" {
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
	if suffix != "memory" && suffix != "rules" {
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
		Scope     string   `json:"scope"`
		Query     string   `json:"query"`
		TopK      int      `json:"top_k"`
		SessionID int64    `json:"session_id"` // optional: auto-resolve team scope
		Tags      []string `json:"tags"`       // optional: post-filter by tags
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scope != "" && !isValidScope(req.Scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be 'memory' or '<groupname>/memory'"})
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
		req.Scope = "memory" // default scope for generic search
	}
	if !waitVectorReady(c) {
		return
	}
	// Fetch extra candidates when filtering by tags
	fetchK := req.TopK
	if len(req.Tags) > 0 {
		fetchK = req.TopK * 3
	}
	results, err := core.Vector.Search(req.Scope, req.Query, fetchK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(req.Tags) > 0 {
		results = filterByTags(results, req.Tags)
		if len(results) > req.TopK {
			results = results[:req.TopK]
		}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// SearchMemory performs semantic search on memory files
// POST /api/v1/vector/search_memory
func SearchMemory(c *gin.Context) {
	vectorSearch(c, "memory")
}

// ListMemoryFiles lists memory files with optional session_id auto-scope.
// GET /api/v1/vector/list_memory?session_id=<id>&scope=<optional>
func ListMemoryFiles(c *gin.Context) {
	vectorList(c, "memory")
}

// detectScopeLevel determines the level from a scope string.
// "memory" → "global", "<group>/memory" → "team", "<group>/sessions/<id>/memory" → "session"
func detectScopeLevel(scope string) string {
	if scope == "memory" {
		return "global"
	}
	parts := strings.Split(scope, "/")
	if len(parts) == 4 && parts[1] == "sessions" {
		return "session"
	}
	return "team"
}

// enrichResult adds type, source_session_id, and level to a vector search result.
// origin: "session" | "team" | "global" (used for sorting priority and level display).
func enrichResult(r map[string]interface{}, scopeType, origin string) map[string]interface{} {
	out := make(map[string]interface{}, len(r)+5)
	for k, v := range r {
		out[k] = v
	}
	out["type"] = scopeType // "memory"
	out["level"] = origin   // "session" | "team" | "global"
	// Extract source_session_id from metadata (stored as float64 in JSON)
	if meta, ok := r["metadata"].(map[string]interface{}); ok {
		if sid, ok := meta["source_session_id"]; ok {
			out["source_session_id"] = sid
		}
		// Add created_at / updated_at from file stat if not already present
		if _, exists := out["created_at"]; !exists {
			if fp, ok := meta["file_path"].(string); ok && fp != "" {
				if info, err := os.Stat(fp); err == nil {
					out["updated_at"] = info.ModTime().Format(time.RFC3339)
					out["created_at"] = fileBirthTime(info).Format(time.RFC3339)
				}
			}
		}
	}
	if _, exists := out["source_session_id"]; !exists {
		out["source_session_id"] = 0
	}
	out["_origin"] = origin // internal sort key, removed before response
	return out
}

// sortPriority returns 0 (self) / 1 (session scope) / 2 (team) / 3 (global) for sorting.
func sortPriority(r map[string]interface{}, sessionID int64) int {
	origin, _ := r["_origin"].(string)
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
	switch origin {
	case "session":
		return 1
	case "team":
		return 2
	default:
		return 3 // global
	}
}

func vectorSearch(c *gin.Context, defaultScope string) {
	var req struct {
		Query     string   `json:"query"`
		TopK      int      `json:"top_k"`
		Scope     string   `json:"scope"`      // optional: explicit scope override
		SessionID int64    `json:"session_id"` // optional: auto-resolve team scope + sorting
		Tags      []string `json:"tags"`       // optional: post-filter by tags
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

	// Determine fetch multiplier for tag filtering
	fetchK := req.TopK
	if len(req.Tags) > 0 {
		fetchK = req.TopK * 3
	}

	// Determine type label from scope suffix
	scopeType := defaultScope // "memory"

	// Explicit scope takes priority
	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		// Derive type from explicit scope suffix and detect level
		parts := strings.Split(req.Scope, "/")
		scopeType = parts[len(parts)-1]
		scopeLevel := detectScopeLevel(req.Scope)

		results, err := core.Vector.Search(req.Scope, req.Query, fetchK)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(req.Tags) > 0 {
			results = filterByTags(results, req.Tags)
		}
		// Merge keyword search results
		seen := make(map[string]bool)
		enriched := make([]map[string]interface{}, 0, len(results))
		for _, r := range results {
			e := enrichResult(r, scopeType, scopeLevel)
			e["origin"] = e["_origin"]
			delete(e, "_origin")
			id, _ := e["id"].(string)
			seen[id] = true
			enriched = append(enriched, e)
		}
		kwResults := core.Vector.KeywordSearch(req.Scope, req.Query, fetchK)
		for _, r := range kwResults {
			id, _ := r["id"].(string)
			if !seen[id] {
				e := enrichResult(r, scopeType, scopeLevel)
				e["origin"] = e["_origin"]
				delete(e, "_origin")
				seen[id] = true
				enriched = append(enriched, e)
			}
		}
		if len(enriched) > req.TopK {
			enriched = enriched[:req.TopK]
		}
		c.JSON(http.StatusOK, gin.H{"results": enriched})
		return
	}

	// Auto-resolve: three-layer merge (session → team → global)
	var sessionGroup string
	if req.SessionID > 0 {
		if sess, err := store.GetSession(req.SessionID); err == nil {
			sessionGroup = sess.GroupName
			if sessionGroup == "" {
				sessionGroup = "_standalone"
			}
		}
	}

	if sessionGroup != "" {
		var merged []map[string]interface{}
		seen := make(map[string]bool)

		// 1. Search session scope
		sessionScope := sessionGroup + "/sessions/" + strconv.FormatInt(req.SessionID, 10) + "/" + defaultScope
		sessionResults, err := core.Vector.Search(sessionScope, req.Query, fetchK)
		if err != nil {
			log.Printf("[vector] session search error (scope=%s): %v", sessionScope, err)
		} else {
			for _, r := range sessionResults {
				e := enrichResult(r, scopeType, "session")
				id, _ := e["id"].(string)
				seen[id] = true
				merged = append(merged, e)
			}
		}
		// Keyword search in session scope
		kwResults := core.Vector.KeywordSearch(sessionScope, req.Query, fetchK)
		for _, r := range kwResults {
			id, _ := r["id"].(string)
			if !seen[id] {
				e := enrichResult(r, scopeType, "session")
				seen[id] = true
				merged = append(merged, e)
			}
		}

		// 2. Search team scope
		teamScope := sessionGroup + "/" + defaultScope
		teamResults, err2 := core.Vector.Search(teamScope, req.Query, fetchK)
		if err2 != nil {
			log.Printf("[vector] team search error (scope=%s): %v", teamScope, err2)
		} else {
			for _, r := range teamResults {
				id, _ := r["id"].(string)
				if !seen[id] {
					e := enrichResult(r, scopeType, "team")
					seen[id] = true
					merged = append(merged, e)
				}
			}
		}
		kwResults = core.Vector.KeywordSearch(teamScope, req.Query, fetchK)
		for _, r := range kwResults {
			id, _ := r["id"].(string)
			if !seen[id] {
				e := enrichResult(r, scopeType, "team")
				seen[id] = true
				merged = append(merged, e)
			}
		}

		// 3. Search global scope
		globalResults, err3 := core.Vector.Search(defaultScope, req.Query, fetchK)
		if err3 != nil {
			log.Printf("[vector] global search error (scope=%s): %v", defaultScope, err3)
		} else {
			for _, r := range globalResults {
				id, _ := r["id"].(string)
				if !seen[id] {
					e := enrichResult(r, scopeType, "global")
					merged = append(merged, e)
					seen[id] = true
				}
			}
		}
		kwResults = core.Vector.KeywordSearch(defaultScope, req.Query, fetchK)
		for _, r := range kwResults {
			id, _ := r["id"].(string)
			if !seen[id] {
				e := enrichResult(r, scopeType, "global")
				merged = append(merged, e)
				seen[id] = true
			}
		}

		// Tag filter before sorting
		if len(req.Tags) > 0 {
			merged = filterByTags(merged, req.Tags)
		}

		// Sort: self(0) > session(1) > team(2) > global(3)
		sortResults(merged, req.SessionID)

		// Trim to topK and remove internal _origin field
		if len(merged) > req.TopK {
			merged = merged[:req.TopK]
		}
		for _, r := range merged {
			r["origin"] = r["_origin"]
			delete(r, "_origin")
		}
		c.JSON(http.StatusOK, gin.H{"results": merged})
		return
	}

	// Default: search global scope only
	results, err := core.Vector.Search(defaultScope, req.Query, fetchK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(req.Tags) > 0 {
		results = filterByTags(results, req.Tags)
	}
	seen := make(map[string]bool)
	enriched := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		e := enrichResult(r, scopeType, "global")
		e["origin"] = e["_origin"]
		delete(e, "_origin")
		id, _ := e["id"].(string)
		seen[id] = true
		enriched = append(enriched, e)
	}
	kwResults := core.Vector.KeywordSearch(defaultScope, req.Query, fetchK)
	for _, r := range kwResults {
		id, _ := r["id"].(string)
		if !seen[id] {
			e := enrichResult(r, scopeType, "global")
			e["origin"] = e["_origin"]
			delete(e, "_origin")
			seen[id] = true
			enriched = append(enriched, e)
		}
	}
	if len(enriched) > req.TopK {
		enriched = enriched[:req.TopK]
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

// filterByTags filters vector results to only include those whose metadata contains
// at least one of the requested tags. Tags in metadata are stored as a JSON-encoded
// string array (e.g. "[\"deploy\",\"sop\"]") due to ChromaDB limitations.
func filterByTags(results []map[string]interface{}, tags []string) []map[string]interface{} {
	if len(tags) == 0 {
		return results
	}
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[strings.TrimSpace(t)] = true
	}

	var filtered []map[string]interface{}
	for _, r := range results {
		meta, ok := r["metadata"].(map[string]interface{})
		if !ok {
			continue
		}
		tagsRaw, ok := meta["mem_tags"]
		if !ok {
			// Also check "tags" key for non-mem records
			tagsRaw, ok = meta["tags"]
			if !ok {
				continue
			}
		}
		// Parse tags: may be JSON string or []interface{} depending on storage
		var docTags []string
		switch v := tagsRaw.(type) {
		case string:
			if err := json.Unmarshal([]byte(v), &docTags); err != nil {
				continue
			}
		case []interface{}:
			for _, t := range v {
				if s, ok := t.(string); ok {
					docTags = append(docTags, s)
				}
			}
		default:
			continue
		}
		for _, dt := range docTags {
			if tagSet[dt] {
				filtered = append(filtered, r)
				break
			}
		}
	}
	return filtered
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
	// Increment read count
	if core.Vector != nil && core.Vector.IsReady() {
		core.Vector.IncrementReadCount(scope, req.FileName)
	}
	c.JSON(http.StatusOK, gin.H{"file_name": req.FileName, "content": string(data), "scope": scope})
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
	vectorWrite(c, "memory")
}

func vectorWrite(c *gin.Context, defaultScope string) {
	var req struct {
		FileName      string                 `json:"file_name"`
		Content       string                 `json:"content"`
		Scope         string                 `json:"scope"`           // optional: explicit scope override
		SessionID     int64                  `json:"session_id"`      // optional: auto-resolve session/team scope
		ExtraMetadata map[string]interface{} `json:"extra_metadata"`  // optional: structured memory fields (tags/type/status/version etc.)
		Schema        string                 `json:"schema"`          // optional: schema name for validation before write
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Schema validation: if schema name is specified, validate content against JSON Schema
	if req.Schema != "" {
		schemaDef, err := store.GetSchema(req.Schema)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load schema: " + err.Error()})
			return
		}
		if schemaDef == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "schema '" + req.Schema + "' not found"})
			return
		}
		if err := validateContentWithSchema(req.Content, schemaDef.Definition); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "schema validation failed: " + err.Error()})
			return
		}
	}

	// Resolve scope with three-layer isolation
	scope := defaultScope
	var sessionGroup string // group of the requesting session
	if req.SessionID > 0 {
		if sess, err := store.GetSession(req.SessionID); err == nil {
			sessionGroup = sess.GroupName
			if sessionGroup == "" {
				sessionGroup = "_standalone"
			}
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

// DeleteMemory deletes a memory file
// POST /api/v1/vector/delete_memory
func DeleteMemory(c *gin.Context) {
	vectorDelete(c, "memory")
}

// DeleteVector deletes a file in any valid vector scope.
// POST /api/v1/vector/delete
func DeleteVector(c *gin.Context) {
	// Default scope is only used when request omits scope.
	vectorDelete(c, "memory")
}

func vectorDelete(c *gin.Context, defaultScope string) {
	var req struct {
		FileName  string `json:"file_name"`
		Scope     string `json:"scope"`      // optional: explicit scope override
		SessionID int64  `json:"session_id"` // optional: auto-resolve session/team scope
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Resolve scope with session-level support (aligned with vectorWrite)
	scope := defaultScope
	var sessionGroup string
	if req.SessionID > 0 {
		if sess, err := store.GetSession(req.SessionID); err == nil {
			sessionGroup = sess.GroupName
			if sessionGroup == "" {
				sessionGroup = "_standalone"
			}
		}
	}

	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scope = req.Scope
	} else if sessionGroup != "" {
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
	os.Remove(path)
	// Clean vector record
	if core.Vector != nil {
		core.Vector.Delete(scope, req.FileName)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName})
}

// StatsVector returns vector hit statistics
// GET /api/v1/vector/stats?scope=memory
func StatsVector(c *gin.Context) {
	scope := c.DefaultQuery("scope", "memory")
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

// fileBirthTime is defined in birthtime_darwin.go / birthtime_other.go

// VectorFileItem represents one file entry in the rich list response.
type VectorFileItem struct {
	FileName        string `json:"file_name"`
	Preview         string `json:"preview"`           // first 100 chars of content
	Type            string `json:"type"`              // "memory"
	SourceSessionID int64  `json:"source_session_id"` // session that wrote this file (0 if unknown)
	CreatedAt       string `json:"created_at"`        // RFC3339 birth time (fallback to mod time)
	UpdatedAt       string `json:"updated_at"`        // RFC3339 mod time
	Scope           string `json:"scope"`
	Origin          string `json:"origin"`            // "session" | "team" | "global"
}

// ListVectorFilesRich lists .md files with preview, type, source_session_id, origin, and updated_at.
// GET /api/v1/vector/list_files?session_id=<id>&scope=<optional>&list_global=<bool>&type=<memory|all>&level=<session|team|global|all>
//
// level parameter:
//   - "session": only session-level files
//   - "team": only team-level files
//   - "global": only global files
//   - "all" (default): all three layers, each tagged with origin
//
// Sorting: self > session > team > global; same level by updated_at desc.
func ListVectorFilesRich(c *gin.Context) {
	sessionIDStr := strings.TrimSpace(c.Query("session_id"))
	explicitScope := strings.TrimSpace(c.Query("scope"))
	listGlobal := c.Query("list_global") == "true"
	typeFilter := strings.TrimSpace(c.Query("type"))   // "memory" | "all" | ""
	levelFilter := strings.TrimSpace(c.Query("level")) // "session" | "team" | "global" | "all" | ""
	tagFilter := strings.TrimSpace(c.Query("tag"))     // optional: filter by tag

	if levelFilter == "" {
		levelFilter = "all"
	}

	var sessionID int64
	if sessionIDStr != "" {
		if id, err := strconv.ParseInt(sessionIDStr, 10, 64); err == nil && id > 0 {
			sessionID = id
		}
	}

	type scopeEntry struct {
		scope  string
		origin string // "session" | "team" | "global"
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
		types := []string{"memory"}
		if typeFilter == "memory" {
			types = []string{"memory"}
		}

		var groupName string
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
		wantGlobal := levelFilter == "all" || levelFilter == "global" || listGlobal

		if groupName != "" {
			if wantSession && sessionID > 0 {
				sidStr := strconv.FormatInt(sessionID, 10)
				for _, t := range types {
					scopesToList = append(scopesToList, scopeEntry{scope: groupName + "/sessions/" + sidStr + "/" + t, origin: "session"})
				}
			}
			if wantTeam {
				for _, t := range types {
					scopesToList = append(scopesToList, scopeEntry{scope: groupName + "/" + t, origin: "team"})
				}
			}
		}
		if wantGlobal || groupName == "" {
			for _, t := range types {
				scopesToList = append(scopesToList, scopeEntry{scope: t, origin: "global"})
			}
		}
	}

	var allItems []VectorFileItem

	for _, se := range scopesToList {
		// Derive type label from scope suffix
		parts := strings.Split(se.scope, "/")
		scopeType := parts[len(parts)-1]

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

			// Tag filter: skip files that don't match the requested tag
			if tagFilter != "" && metaMap != nil {
				if meta, ok := metaMap[e.Name()]; ok {
					matched := false
					for _, key := range []string{"mem_tags", "tags"} {
						if tagsRaw, ok := meta[key]; ok {
							var docTags []string
							switch v := tagsRaw.(type) {
							case string:
								json.Unmarshal([]byte(v), &docTags)
							case []interface{}:
								for _, t := range v {
									if s, ok := t.(string); ok {
										docTags = append(docTags, s)
									}
								}
							}
							for _, dt := range docTags {
								if dt == tagFilter {
									matched = true
									break
								}
							}
						}
						if matched {
							break
						}
					}
					if !matched {
						continue
					}
				} else {
					continue // no metadata = no tags = skip
				}
			}

			allItems = append(allItems, VectorFileItem{
				FileName:        e.Name(),
				Preview:         preview,
				Type:            scopeType,
				SourceSessionID: sourceSessionID,
				CreatedAt:       fileBirthTime(info).Format(time.RFC3339),
				UpdatedAt:       info.ModTime().Format(time.RFC3339),
				Scope:           se.scope,
				Origin:          se.origin,
			})
		}
	}

	// Sort: self(0) > session(1) > team(2) > global(3); same level by updated_at desc
	filePriority := func(item VectorFileItem) int {
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
	// Increment read count
	if core.Vector != nil && core.Vector.IsReady() {
		core.Vector.IncrementReadCount(req.Scope, req.FileName)
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

// validateContentWithSchema validates content against a JSON Schema definition.
// Content can be:
//   - A JSON string: validated directly against the schema
//   - Non-JSON (e.g. markdown): wrapped as {"content": "<text>"} and validated
func validateContentWithSchema(content, schemaDef string) error {
	// Parse the JSON Schema definition into a Go value
	var schemaDoc interface{}
	if err := json.Unmarshal([]byte(schemaDef), &schemaDoc); err != nil {
		return fmt.Errorf("invalid schema definition: %w", err)
	}

	// Create compiler and add the schema as a resource (use virtual URI to avoid leaking server paths)
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("mem://schema", schemaDoc); err != nil {
		return fmt.Errorf("failed to add schema resource: %w", err)
	}
	sch, err := compiler.Compile("mem://schema")
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	// Try to parse content as JSON first
	var doc interface{}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		// Not valid JSON — wrap as {"content": "..."}
		doc = map[string]interface{}{"content": content}
	}

	// Validate
	if err := sch.Validate(doc); err != nil {
		return err
	}
	return nil
}
