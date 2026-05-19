package api

import (
	"ai-hub/server/core"
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var dataDir string

func InitDataDir(dir string) {
	dataDir = dir
	// Initialize hook stream callback (Issue #211)
	initHookStreamCallback()
}

type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	WhenToUse   string `json:"when_to_use,omitempty"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
}

type ToggleSkillRequest struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Enable bool   `json:"enable"`
}

type SkillImportCandidate struct {
	ID          string   `json:"id"`
	DirName     string   `json:"dir_name"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	WhenToUse   string   `json:"when_to_use,omitempty"`
	FileCount   int      `json:"file_count"`
	Files       []string `json:"files"`
	Exists      bool     `json:"exists"`
}

type SkillImportPreviewResponse struct {
	ArchiveName string                 `json:"archive_name"`
	Mode        string                 `json:"mode"`
	Candidates  []SkillImportCandidate `json:"candidates"`
	Warnings    []string               `json:"warnings"`
}

type SkillImportConfirmRequest struct {
	Skills    []string `form:"skills"`
	Overwrite bool     `form:"overwrite"`
}

func parseSkillFrontmatter(path string) (name, desc, whenToUse string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", ""
	}
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return "", "", ""
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return "", "", ""
	}
	fm := content[3 : 3+end]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, "\"'")
		} else if strings.HasPrefix(line, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			desc = strings.Trim(desc, "\"'")
		} else if strings.HasPrefix(line, "when_to_use:") {
			whenToUse = strings.TrimSpace(strings.TrimPrefix(line, "when_to_use:"))
			whenToUse = strings.Trim(whenToUse, "\"'")
		}
	}
	return
}

func disabledSkillPath(name, source string) string {
	base := filepath.Join(dataDir, "disabled", "skills", source)
	return filepath.Join(base, name)
}

func disabledCommandPath(name string) string {
	return filepath.Join(dataDir, "disabled", "commands", name+".md")
}

func isSkillDisabled(name, source string) bool {
	if source == "command" {
		_, err := os.Stat(disabledCommandPath(name))
		return err == nil
	}
	_, err := os.Stat(disabledSkillPath(name, source))
	return err == nil
}

func scanUserSkills() []SkillInfo {
	dir := filepath.Join(core.GetDataDir(), "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var skills []SkillInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		name, desc, whenToUse := parseSkillFrontmatter(skillFile)
		if name == "" {
			name = e.Name()
		}
		skills = append(skills, SkillInfo{
			Name:        name,
			Description: desc,
			WhenToUse:   whenToUse,
			Source:      "user",
			Path:        skillFile,
			Enabled:     !isSkillDisabled(e.Name(), "user"),
		})
	}
	return skills
}

func scanPluginSkills() []SkillInfo {
	base := filepath.Join(core.GetDataDir(), "plugins", "marketplaces")
	marketplaces, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var skills []SkillInfo
	for _, m := range marketplaces {
		if !m.IsDir() {
			continue
		}
		pluginsDir := filepath.Join(base, m.Name(), "plugins")
		plugins, err := os.ReadDir(pluginsDir)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			if !p.IsDir() {
				continue
			}
			skillsDir := filepath.Join(pluginsDir, p.Name(), "skills")
			skillEntries, err := os.ReadDir(skillsDir)
			if err != nil {
				continue
			}
			for _, s := range skillEntries {
				if !s.IsDir() {
					continue
				}
				skillFile := filepath.Join(skillsDir, s.Name(), "SKILL.md")
				if _, err := os.Stat(skillFile); err != nil {
					continue
				}
				name, desc, whenToUse := parseSkillFrontmatter(skillFile)
				if name == "" {
					name = s.Name()
				}
				skills = append(skills, SkillInfo{
					Name:        name,
					Description: desc,
					WhenToUse:   whenToUse,
					Source:      "plugin",
					Path:        skillFile,
					Enabled:     !isSkillDisabled(s.Name(), "plugin"),
				})
			}
		}
	}
	return skills
}

func scanCommands() []SkillInfo {
	dir := filepath.Join(core.GetDataDir(), "commands")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var skills []SkillInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		skills = append(skills, SkillInfo{
			Name:        name,
			Description: "斜杠命令 /" + name,
			Source:      "command",
			Path:        filepath.Join(dir, e.Name()),
			Enabled:     !isSkillDisabled(name, "command"),
		})
	}
	return skills
}

func ListSkills(c *gin.Context) {
	var all []SkillInfo
	all = append(all, scanUserSkills()...)
	all = append(all, scanPluginSkills()...)
	all = append(all, scanCommands()...)
	if all == nil {
		all = []SkillInfo{}
	}
	c.JSON(http.StatusOK, all)
}

func ToggleSkill(c *gin.Context) {
	var req ToggleSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Source == "command" {
		disPath := disabledCommandPath(req.Name)
		origPath := filepath.Join(core.GetDataDir(), "commands", req.Name+".md")
		if req.Enable {
			// Move back from disabled
			if _, err := os.Stat(disPath); err == nil {
				os.MkdirAll(filepath.Dir(origPath), 0755)
				os.Rename(disPath, origPath)
			}
		} else {
			// Move to disabled
			if _, err := os.Stat(origPath); err == nil {
				os.MkdirAll(filepath.Dir(disPath), 0755)
				os.Rename(origPath, disPath)
			}
		}
	} else {
		// Find original path by resolving display name to directory name
		var dirName string
		var origDir string
		if req.Source == "user" {
			dirName = resolveSkillDirName(req.Name)
			if dirName != "" {
				origDir = filepath.Join(core.GetDataDir(), "skills", dirName)
			}
		} else {
			origDir = findPluginSkillDir(req.Name)
			if origDir != "" {
				dirName = filepath.Base(origDir)
			}
		}
		disPath := disabledSkillPath(dirName, req.Source)
		if req.Enable {
			if _, err := os.Stat(disPath); err == nil && origDir != "" {
				os.MkdirAll(filepath.Dir(origDir), 0755)
				os.Rename(disPath, origDir)
			}
		} else {
			if origDir != "" {
				if _, err := os.Stat(origDir); err == nil {
					os.MkdirAll(filepath.Dir(disPath), 0755)
					os.Rename(origDir, disPath)
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// resolveSkillDirName finds the directory name for a skill by its display name.
// Display name (from frontmatter) may differ from directory name.
// Scans both active and disabled skill directories.
func resolveSkillDirName(displayName string) string {
	// 1. Scan active skills
	dir := filepath.Join(core.GetDataDir(), "skills")
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
			name, _, _ := parseSkillFrontmatter(skillFile)
			if name == displayName || e.Name() == displayName {
				return e.Name()
			}
		}
	}
	// 2. Scan disabled skills (for re-enable: skill is not in active dir)
	disDir := filepath.Join(core.GetDataDir(), "disabled", "skills", "user")
	disEntries, err := os.ReadDir(disDir)
	if err == nil {
		for _, e := range disEntries {
			if !e.IsDir() {
				continue
			}
			skillFile := filepath.Join(disDir, e.Name(), "SKILL.md")
			name, _, _ := parseSkillFrontmatter(skillFile)
			if name == displayName || e.Name() == displayName {
				return e.Name()
			}
		}
	}
	return ""
}

// GetSkillContent reads the full SKILL.md content for a skill
func GetSkillContent(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// Try to resolve display name to dir name
	dirName := resolveSkillDirName(name)
	if dirName == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	skillFile := filepath.Join(core.GetDataDir(), "skills", dirName, "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill file not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":    name,
		"dir":     dirName,
		"content": string(data),
	})
}

// CreateSkillRequest is the request body for creating a skill
type CreateSkillRequest struct {
	Name    string `json:"name" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// CreateSkill creates a new user skill
func CreateSkill(c *gin.Context) {
	var req CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Sanitize name: use as directory name
	dirName := sanitizeSkillName(req.Name)
	if dirName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid skill name"})
		return
	}

	skillDir := filepath.Join(core.GetDataDir(), "skills", dirName)
	if _, err := os.Stat(skillDir); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "skill already exists"})
		return
	}

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create skill directory"})
		return
	}

	// Auto-wrap with frontmatter if missing (Claude Code requires frontmatter for skill discovery)
	content := req.Content
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		content = fmt.Sprintf("---\nname: %q\ndescription: %q\n---\n\n%s", req.Name, req.Name, content)
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(content), 0644); err != nil {
		os.RemoveAll(skillDir)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write skill file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "dir": dirName})
}

// UpdateSkillRequest is the request body for updating a skill
type UpdateSkillRequest struct {
	Content string `json:"content" binding:"required"`
}

// UpdateSkill updates an existing user skill's SKILL.md
func UpdateSkill(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	var req UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dirName := resolveSkillDirName(name)
	if dirName == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	skillFile := filepath.Join(core.GetDataDir(), "skills", dirName, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write skill file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteSkill deletes a user skill directory
func DeleteSkill(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	dirName := resolveSkillDirName(name)
	if dirName == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	skillDir := filepath.Join(core.GetDataDir(), "skills", dirName)
	if err := os.RemoveAll(skillDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete skill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func safeZipPath(name string) (string, bool) {
	name = filepath.ToSlash(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.Contains(name, "..") || filepath.IsAbs(name) || strings.HasPrefix(name, "__MACOSX/") {
		return "", false
	}
	return name, true
}

func zipRootEntries(files map[string][]byte) []string {
	set := map[string]bool{}
	for p := range files {
		parts := strings.Split(p, "/")
		if parts[0] != "" {
			set[parts[0]] = true
		}
	}
	var roots []string
	for r := range set {
		roots = append(roots, r)
	}
	sort.Strings(roots)
	return roots
}

func stripCommonRoot(files map[string][]byte) map[string][]byte {
	roots := zipRootEntries(files)
	if len(roots) != 1 {
		return files
	}
	root := roots[0]
	prefix := root + "/"
	for p := range files {
		if p == root || strings.HasPrefix(p, prefix) {
			continue
		}
		return files
	}
	stripped := map[string][]byte{}
	for p, data := range files {
		if p == root {
			continue
		}
		stripped[strings.TrimPrefix(p, prefix)] = data
	}
	if len(stripped) == 0 {
		return files
	}
	return stripped
}

func readSkillZip(file io.ReaderAt, size int64, archiveName string) (SkillImportPreviewResponse, map[string]map[string][]byte, error) {
	zr, err := zip.NewReader(file, size)
	if err != nil {
		return SkillImportPreviewResponse{}, nil, fmt.Errorf("invalid zip format")
	}
	allFiles := map[string][]byte{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, ok := safeZipPath(f.Name)
		if !ok {
			continue
		}
		if f.UncompressedSize64 > 5*1024*1024 {
			return SkillImportPreviewResponse{}, nil, fmt.Errorf("file too large: %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return SkillImportPreviewResponse{}, nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 5*1024*1024+1))
		rc.Close()
		if err != nil {
			return SkillImportPreviewResponse{}, nil, err
		}
		allFiles[name] = data
	}
	if len(allFiles) == 0 {
		return SkillImportPreviewResponse{}, nil, fmt.Errorf("archive has no files")
	}

	baseName := strings.TrimSuffix(filepath.Base(archiveName), filepath.Ext(archiveName))
	baseName = sanitizeSkillName(baseName)
	if baseName == "" {
		baseName = fmt.Sprintf("skill-%d", time.Now().Unix())
	}

	preview := SkillImportPreviewResponse{ArchiveName: archiveName, Warnings: []string{}}
	skillFiles := []string{}
	for p := range allFiles {
		if strings.EqualFold(filepath.Base(p), "SKILL.md") {
			skillFiles = append(skillFiles, p)
		}
	}
	sort.Strings(skillFiles)
	if len(skillFiles) == 0 {
		return SkillImportPreviewResponse{}, nil, fmt.Errorf("SKILL.md not found in archive")
	}

	bundles := map[string]map[string][]byte{}
	if _, ok := allFiles["SKILL.md"]; ok {
		preview.Mode = "single-root-file"
		bundles[baseName] = allFiles
	} else {
		rootsWithSkill := map[string]bool{}
		for _, sf := range skillFiles {
			parts := strings.Split(sf, "/")
			if len(parts) > 1 {
				rootsWithSkill[parts[0]] = true
			}
		}
		if len(rootsWithSkill) == 1 {
			var root string
			for r := range rootsWithSkill {
				root = r
			}
			prefix := root + "/"
			sub := map[string][]byte{}
			for p, data := range allFiles {
				if strings.HasPrefix(p, prefix) {
					sub[strings.TrimPrefix(p, prefix)] = data
				}
			}
			dirName := sanitizeSkillName(root)
			if dirName == "" {
				dirName = baseName
			}
			preview.Mode = "single-folder"
			bundles[dirName] = sub
		} else {
			preview.Mode = "multi-skill"
			for root := range rootsWithSkill {
				prefix := root + "/"
				sub := map[string][]byte{}
				for p, data := range allFiles {
					if strings.HasPrefix(p, prefix) {
						sub[strings.TrimPrefix(p, prefix)] = data
					}
				}
				dirName := sanitizeSkillName(root)
				if dirName == "" {
					dirName = root
				}
				bundles[dirName] = sub
			}
		}
	}

	for dir, files := range bundles {
		files = stripCommonRoot(files)
		bundles[dir] = files
		skillData, ok := files["SKILL.md"]
		if !ok {
			preview.Warnings = append(preview.Warnings, fmt.Sprintf("%s 缺少 SKILL.md，已跳过", dir))
			delete(bundles, dir)
			continue
		}
		tmp, err := os.CreateTemp("", "skill-import-*.md")
		name, desc, when := dir, "", ""
		if err == nil {
			tmp.Write(skillData)
			tmp.Close()
			name, desc, when = parseSkillFrontmatter(tmp.Name())
			os.Remove(tmp.Name())
		}
		if name == "" {
			name = dir
		}
		fileList := make([]string, 0, len(files))
		for p := range files {
			fileList = append(fileList, p)
		}
		sort.Strings(fileList)
		if len(fileList) > 20 {
			fileList = fileList[:20]
		}
		preview.Candidates = append(preview.Candidates, SkillImportCandidate{
			ID: dir, DirName: dir, Name: name, Description: desc, WhenToUse: when,
			FileCount: len(files), Files: fileList,
			Exists: dirExists(filepath.Join(core.GetDataDir(), "skills", dir)),
		})
	}
	sort.Slice(preview.Candidates, func(i, j int) bool { return preview.Candidates[i].DirName < preview.Candidates[j].DirName })
	if len(preview.Candidates) == 0 {
		return SkillImportPreviewResponse{}, nil, fmt.Errorf("no valid skill found")
	}
	return preview, bundles, nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func writeSkillBundle(destDir string, files map[string][]byte, overwrite bool) error {
	if dirExists(destDir) {
		if !overwrite {
			return fmt.Errorf("skill already exists: %s", filepath.Base(destDir))
		}
		if err := os.RemoveAll(destDir); err != nil {
			return err
		}
	}
	for rel, data := range files {
		rel, ok := safeZipPath(rel)
		if !ok {
			continue
		}
		dest := filepath.Join(destDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(dest, destDir+string(os.PathSeparator)) && dest != destDir {
			return fmt.Errorf("invalid path: %s", rel)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func ExportSkill(c *gin.Context) {
	name := c.Param("name")
	dirName := resolveSkillDirName(name)
	if dirName == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	skillDir := filepath.Join(core.GetDataDir(), "skills", dirName)
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	err := filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(skillDir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(filepath.Join(dirName, rel))
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export skill"})
		return
	}
	filename := sanitizeSkillName(dirName) + ".zip"
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func PreviewSkillImport(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open file"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 50*1024*1024+1))
	if err != nil || len(data) > 50*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "archive too large"})
		return
	}
	preview, _, err := readSkillZip(bytes.NewReader(data), int64(len(data)), fh.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, preview)
}

func ImportSkills(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	selected := c.PostFormArray("skills")
	overwrite := c.PostForm("overwrite") == "true"
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open file"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 50*1024*1024+1))
	if err != nil || len(data) > 50*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "archive too large"})
		return
	}
	preview, bundles, err := readSkillZip(bytes.NewReader(data), int64(len(data)), fh.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selectedSet := map[string]bool{}
	if len(selected) == 0 && len(preview.Candidates) == 1 {
		selectedSet[preview.Candidates[0].ID] = true
	} else {
		for _, id := range selected {
			selectedSet[id] = true
		}
	}
	if len(selectedSet) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no skill selected"})
		return
	}
	imported := []string{}
	warnings := append([]string{}, preview.Warnings...)
	for _, cand := range preview.Candidates {
		if !selectedSet[cand.ID] {
			continue
		}
		files := bundles[cand.ID]
		if files == nil {
			continue
		}
		dest := filepath.Join(core.GetDataDir(), "skills", cand.DirName)
		if err := writeSkillBundle(dest, files, overwrite); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		imported = append(imported, cand.DirName)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "imported": imported, "warnings": warnings})
}

// sanitizeSkillName converts a skill name to a safe directory name
func sanitizeSkillName(name string) string {
	// Replace spaces and special chars with hyphens
	name = strings.ToLower(strings.TrimSpace(name))
	var result []byte
	for _, ch := range []byte(name) {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			result = append(result, ch)
		} else if ch == ' ' {
			result = append(result, '-')
		}
		// skip other chars (including multi-byte UTF-8 leading bytes for CJK)
	}
	// For CJK names, allow the original name if ASCII sanitization produces empty
	s := strings.Trim(string(result), "-")
	if s == "" {
		// Fallback: use original trimmed name as-is (supports CJK directory names)
		return strings.TrimSpace(name)
	}
	return s
}

func findPluginSkillDir(name string) string {
	base := filepath.Join(core.GetDataDir(), "plugins", "marketplaces")
	marketplaces, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, m := range marketplaces {
		if !m.IsDir() {
			continue
		}
		pluginsDir := filepath.Join(base, m.Name(), "plugins")
		plugins, _ := os.ReadDir(pluginsDir)
		for _, p := range plugins {
			if !p.IsDir() {
				continue
			}
			skillDir := filepath.Join(pluginsDir, p.Name(), "skills", name)
			if _, err := os.Stat(skillDir); err == nil {
				return skillDir
			}
			// Also check by frontmatter display name
			skillEntries, _ := os.ReadDir(filepath.Join(pluginsDir, p.Name(), "skills"))
			for _, s := range skillEntries {
				if !s.IsDir() {
					continue
				}
				skillFile := filepath.Join(pluginsDir, p.Name(), "skills", s.Name(), "SKILL.md")
				fmName, _, _ := parseSkillFrontmatter(skillFile)
				if fmName == name {
					return filepath.Join(pluginsDir, p.Name(), "skills", s.Name())
				}
			}
		}
	}
	return ""
}
