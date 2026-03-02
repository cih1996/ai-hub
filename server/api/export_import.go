package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
)

// --- Data structures for export archive ---

type exportManifest struct {
	Version      string  `json:"version"`
	ExportType   string  `json:"export_type"` // "session" | "team"
	Name         string  `json:"name"`
	GroupName    string  `json:"group_name,omitempty"`
	SessionIDs   []int64 `json:"session_ids"`
	ExportedAt   string  `json:"exported_at"`
	AIHubVersion string  `json:"ai_hub_version"`
}

type sessionExportData struct {
	Session  model.Session   `json:"session"`
	Messages []model.Message `json:"messages"`
}

// --- Export handlers ---

// ExportSession exports a single session as tar.gz
// GET /api/v1/export/session/:id
func ExportSession(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	session, err := store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	msgs, _ := store.GetMessages(id)
	rules, _ := ReadSessionRules(id)

	sessions := []sessionExportData{{Session: *session, Messages: msgs}}
	manifest := exportManifest{
		Version:      "1.0",
		ExportType:   "session",
		Name:         session.Title,
		GroupName:    session.GroupName,
		SessionIDs:   []int64{id},
		ExportedAt:   time.Now().Format(time.RFC3339),
		AIHubVersion: appVersion,
	}
	rulesMap := map[int64]string{}
	if rules != "" {
		rulesMap[id] = rules
	}

	// Determine team dir (only if session belongs to a team)
	teamDir := ""
	if session.GroupName != "" {
		td := core.TeamDir(session.GroupName)
		if info, err2 := os.Stat(td); err2 == nil && info.IsDir() {
			teamDir = td
		}
	}

	filename := sanitizeFilename(fmt.Sprintf("export-session-%s-%s.tar.gz", session.Title, time.Now().Format("20060102150405")))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/gzip")
	if err := buildTarGz(c.Writer, manifest, sessions, rulesMap, teamDir); err != nil {
		log.Printf("[export] session %d error: %v", id, err)
	}
}

// ExportTeam exports all sessions and resources of a team as tar.gz
// GET /api/v1/export/team/:name
func ExportTeam(c *gin.Context) {
	name := c.Param("name")
	if !isValidGroupName(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team name"})
		return
	}
	sessions, err := store.ListSessionsByGroup(name)
	if err != nil || len(sessions) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no sessions found for team"})
		return
	}

	var exportSessions []sessionExportData
	var sessionIDs []int64
	rulesMap := map[int64]string{}
	for _, s := range sessions {
		msgs, _ := store.GetMessages(s.ID)
		exportSessions = append(exportSessions, sessionExportData{Session: s, Messages: msgs})
		sessionIDs = append(sessionIDs, s.ID)
		if rules, _ := ReadSessionRules(s.ID); rules != "" {
			rulesMap[s.ID] = rules
		}
	}

	manifest := exportManifest{
		Version:      "1.0",
		ExportType:   "team",
		Name:         name,
		GroupName:    name,
		SessionIDs:   sessionIDs,
		ExportedAt:   time.Now().Format(time.RFC3339),
		AIHubVersion: appVersion,
	}

	teamDir := core.TeamDir(name)
	if info, err2 := os.Stat(teamDir); err2 != nil || !info.IsDir() {
		teamDir = ""
	}

	filename := sanitizeFilename(fmt.Sprintf("export-team-%s-%s.tar.gz", name, time.Now().Format("20060102150405")))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/gzip")
	if err := buildTarGz(c.Writer, manifest, exportSessions, rulesMap, teamDir); err != nil {
		log.Printf("[export] team %s error: %v", name, err)
	}
}

// --- Import handler ---

// ImportArchive imports a tar.gz export archive
// POST /api/v1/import
func ImportArchive(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	// Size guard: 100MB
	if header.Size > 100*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file too large (max 100MB)"})
		return
	}

	gz, err := gzip.NewReader(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gzip format"})
		return
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	// Accumulate archive contents in memory
	var manifest *exportManifest
	sessionInfos := map[string]*sessionExportData{} // "sessions/<id>/info.json" key=<id>
	sessionRules := map[string]string{}              // key=<id>, value=rules content
	teamFiles := map[string][]byte{}                 // key="team/knowledge/file.md", value=content
	var totalSize int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tar format: " + err.Error()})
			return
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		totalSize += hdr.Size
		if totalSize > 200*1024*1024 {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "archive content too large"})
			return
		}
		data, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil {
			continue
		}

		name := hdr.Name
		switch {
		case name == "manifest.json":
			var m exportManifest
			if err := json.Unmarshal(data, &m); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid manifest.json"})
				return
			}
			manifest = &m

		case strings.HasSuffix(name, "/info.json") && strings.HasPrefix(name, "sessions/"):
			// Extract session ID from path: sessions/<id>/info.json
			parts := strings.Split(name, "/")
			if len(parts) == 3 {
				var sd sessionExportData
				if err := json.Unmarshal(data, &sd); err == nil {
					sessionInfos[parts[1]] = &sd
				}
			}

		case strings.HasSuffix(name, "/role.md") && strings.HasPrefix(name, "sessions/"):
			parts := strings.Split(name, "/")
			if len(parts) == 3 && len(data) > 0 {
				sessionRules[parts[1]] = string(data)
			}

		case strings.HasPrefix(name, "team/"):
			teamFiles[name] = data
		}
	}

	if manifest == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manifest.json not found in archive"})
		return
	}

	// Import sessions
	idMap := map[string]int64{}
	sessionsImported := 0
	for oldID, sd := range sessionInfos {
		newSession := &model.Session{
			Title:     sd.Session.Title,
			WorkDir:   sd.Session.WorkDir,
			GroupName: sd.Session.GroupName,
		}
		// Find a valid provider ID, fallback to empty (system will use default)
		newSession.ProviderID = sd.Session.ProviderID
		if err := store.CreateSession(newSession); err != nil {
			log.Printf("[import] failed to create session (old id=%s): %v", oldID, err)
			continue
		}
		idMap[oldID] = newSession.ID

		// Import messages
		for _, msg := range sd.Messages {
			m := &model.Message{
				SessionID: newSession.ID,
				Role:      msg.Role,
				Content:   msg.Content,
				Metadata:  msg.Metadata,
			}
			store.AddMessage(m)
		}

		// Import session rules
		if rules, ok := sessionRules[oldID]; ok && rules != "" {
			os.MkdirAll(sessionRulesDir(), 0755)
			os.WriteFile(sessionRulesPath(newSession.ID), []byte(rules), 0644)
		}

		sessionsImported++
	}

	// Import team files
	teamFilesImported := 0
	var warnings []string
	if len(teamFiles) > 0 && manifest.GroupName != "" {
		teamBase := core.TeamDir(manifest.GroupName)
		for archivePath, data := range teamFiles {
			// archivePath is like "team/knowledge/file.md"
			relPath := strings.TrimPrefix(archivePath, "team/")
			destPath := filepath.Join(teamBase, relPath)
			destDir := filepath.Dir(destPath)

			// Safety check
			if !strings.HasPrefix(destPath, teamBase) {
				continue
			}

			// Skip if file already exists (merge-without-overwrite)
			if _, err2 := os.Stat(destPath); err2 == nil {
				warnings = append(warnings, fmt.Sprintf("file %s already exists, skipped", relPath))
				continue
			}

			os.MkdirAll(destDir, 0755)
			if err := os.WriteFile(destPath, data, 0644); err != nil {
				log.Printf("[import] failed to write team file %s: %v", destPath, err)
				continue
			}
			teamFilesImported++

			// Trigger vector sync for knowledge/memory files
			parts := strings.SplitN(relPath, "/", 2)
			if len(parts) == 2 && (parts[0] == "knowledge" || parts[0] == "memory") {
				scope := manifest.GroupName + "/" + parts[0]
				core.SyncFileToVector(scope, destPath)
			}
		}
		if teamFilesImported > 0 || len(warnings) > 0 {
			log.Printf("[import] team '%s': %d files imported, %d skipped", manifest.GroupName, teamFilesImported, len(warnings))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":                  true,
		"sessions_imported":   sessionsImported,
		"session_id_map":      idMap,
		"team_files_imported": teamFilesImported,
		"warnings":            warnings,
	})
}

// --- Shared helpers ---

// buildTarGz writes a complete export archive to w.
func buildTarGz(w io.Writer, manifest exportManifest, sessions []sessionExportData, rulesMap map[int64]string, teamDir string) error {
	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Write manifest.json
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := writeTarFile(tw, "manifest.json", manifestData); err != nil {
		return err
	}

	// Write README.md
	readme := fmt.Sprintf("# AI Hub Export\n\nType: %s\nName: %s\nExported: %s\nVersion: %s\nSessions: %d\n",
		manifest.ExportType, manifest.Name, manifest.ExportedAt, manifest.AIHubVersion, len(sessions))
	if err := writeTarFile(tw, "README.md", []byte(readme)); err != nil {
		return err
	}

	// Write sessions
	for _, sd := range sessions {
		idStr := strconv.FormatInt(sd.Session.ID, 10)
		prefix := "sessions/" + idStr + "/"

		infoData, _ := json.MarshalIndent(sd, "", "  ")
		if err := writeTarFile(tw, prefix+"info.json", infoData); err != nil {
			return err
		}

		if rules, ok := rulesMap[sd.Session.ID]; ok {
			if err := writeTarFile(tw, prefix+"role.md", []byte(rules)); err != nil {
				return err
			}
		}
	}

	// Write team files
	if teamDir != "" {
		for _, sub := range []string{"knowledge", "memory", "rules"} {
			subDir := filepath.Join(teamDir, sub)
			entries, err := os.ReadDir(subDir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(subDir, e.Name()))
				if err != nil {
					continue
				}
				if err := writeTarFile(tw, "team/"+sub+"/"+e.Name(), data); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func writeTarFile(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func sanitizeFilename(name string) string {
	// Replace problematic chars with underscore
	re := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	name = re.ReplaceAllString(name, "_")
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

func isValidGroupName(name string) bool {
	if name == "" || len(name) > 200 {
		return false
	}
	if strings.Contains(name, "..") || strings.Contains(name, "\x00") || strings.Contains(name, "/") {
		return false
	}
	for _, ch := range name {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '-' && ch != '_' && ch != ' ' {
			return false
		}
	}
	return true
}
