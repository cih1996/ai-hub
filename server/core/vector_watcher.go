package core

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// VectorWatcher polls memory/ directories for changes
// and syncs files to the vector engine.
type VectorWatcher struct {
	mu       sync.Mutex
	stopCh   chan struct{}
	snapshots map[string]fileSnapshot // path -> snapshot
	dirs     map[string]string       // dir path -> scope
}

type fileSnapshot struct {
	modTime time.Time
	size    int64
}

// StartVectorWatcher begins polling watched directories.
// Also discovers existing team-level memory dirs under ~/.ai-hub/teams/<groupname>/.
func StartVectorWatcher() *VectorWatcher {
	home, _ := os.UserHomeDir()
	dirs := map[string]string{
		filepath.Join(home, ".ai-hub", "memory"): "memory",
	}
	// Discover existing team-level memory directories under ~/.ai-hub/teams/
	teamsDir := filepath.Join(home, ".ai-hub", "teams")
	if entries, err := os.ReadDir(teamsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			groupName := e.Name()
			for _, sub := range []string{"memory"} {
				subDir := filepath.Join(teamsDir, groupName, sub)
				if info, err2 := os.Stat(subDir); err2 == nil && info.IsDir() {
					scope := groupName + "/" + sub
					dirs[subDir] = scope
					log.Printf("[vector-watcher] discovered team dir: %s -> scope=%s", subDir, scope)
				}
			}
			// Discover session-level directories: teams/<group>/sessions/<id>/<sub>
			sessionsDir := filepath.Join(teamsDir, groupName, "sessions")
			if sessionEntries, err2 := os.ReadDir(sessionsDir); err2 == nil {
				for _, se := range sessionEntries {
					if !se.IsDir() {
						continue
					}
					for _, sub := range []string{"memory"} {
						subDir := filepath.Join(sessionsDir, se.Name(), sub)
						if info, err3 := os.Stat(subDir); err3 == nil && info.IsDir() {
							scope := groupName + "/sessions/" + se.Name() + "/" + sub
							dirs[subDir] = scope
							log.Printf("[vector-watcher] discovered session dir: %s -> scope=%s", subDir, scope)
						}
					}
				}
			}
		}
	}
	w := &VectorWatcher{
		stopCh:    make(chan struct{}),
		snapshots: make(map[string]fileSnapshot),
		dirs:      dirs,
	}
	go w.loop()
	log.Println("[vector-watcher] started")
	return w
}

// AddWatchDir dynamically registers a new directory for polling.
// Called when a file is first written to a new team scope via the API.
func (w *VectorWatcher) AddWatchDir(dir, scope string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.dirs[dir]; !exists {
		w.dirs[dir] = scope
		log.Printf("[vector-watcher] added watch dir: %s -> scope=%s", dir, scope)
	}
}

func (w *VectorWatcher) Stop() {
	close(w.stopCh)
	log.Println("[vector-watcher] stopped")
}

func (w *VectorWatcher) loop() {
	// Initial full sync
	w.fullSync()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			if Vector == nil || !Vector.IsReady() {
				continue
			}
			w.poll()
		}
	}
}

func (w *VectorWatcher) fullSync() {
	if Vector == nil || !Vector.IsReady() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	// Fetch existing metadata per scope once, to preserve source_session_id on restart.
	// Prevents overwriting valid session IDs with 0 when re-embedding existing files.
	scopeMeta := make(map[string]map[string]map[string]interface{})
	for _, scope := range w.dirs {
		if _, fetched := scopeMeta[scope]; !fetched {
			scopeMeta[scope] = Vector.ListMetadata(scope) // nil if unavailable
		}
	}

	for dir, scope := range w.dirs {
		os.MkdirAll(dir, 0755)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		localFiles := make(map[string]bool)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			localFiles[e.Name()] = true
			path := filepath.Join(dir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}
			w.snapshots[path] = fileSnapshot{modTime: info.ModTime(), size: info.Size()}
			// Preserve existing source_session_id if available in vector DB
			sessionID := extractSessionID(scopeMeta[scope], e.Name())
			syncFileToVector(scope, path, sessionID)
		}
		// Reverse cleanup: delete vector records that have no corresponding local file
		if meta := scopeMeta[scope]; meta != nil {
			for docID := range meta {
				if !localFiles[docID] {
					if err := Vector.Delete(scope, docID); err != nil {
						log.Printf("[vector-watcher] orphan delete error %s/%s: %v", scope, docID, err)
					} else {
						log.Printf("[vector-watcher] orphan deleted: %s/%s", scope, docID)
					}
				}
			}
		}
	}
	log.Printf("[vector-watcher] initial sync: %d files", len(w.snapshots))
}

func (w *VectorWatcher) poll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	currentFiles := make(map[string]bool)
	// Lazy per-scope metadata cache: only fetched when a changed file is found.
	// Preserves source_session_id for files modified outside the API (e.g., direct FS writes).
	scopeMetaCache := make(map[string]map[string]map[string]interface{})

	for dir, scope := range w.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			currentFiles[path] = true

			info, err := e.Info()
			if err != nil {
				continue
			}
			snap, exists := w.snapshots[path]
			if !exists || info.ModTime().After(snap.modTime) || info.Size() != snap.size {
				w.snapshots[path] = fileSnapshot{modTime: info.ModTime(), size: info.Size()}
				// Lazy-fetch scope metadata once per scope per poll cycle
				if _, fetched := scopeMetaCache[scope]; !fetched {
					scopeMetaCache[scope] = Vector.ListMetadata(scope)
				}
				sessionID := extractSessionID(scopeMetaCache[scope], e.Name())
				syncFileToVector(scope, path, sessionID)
			}
		}
	}

	// detect deletions
	for path := range w.snapshots {
		if !currentFiles[path] {
			scope := w.scopeForPath(path)
			delete(w.snapshots, path)
			if scope != "" {
				docID := filepath.Base(path)
				if err := Vector.Delete(scope, docID); err != nil {
					log.Printf("[vector-watcher] delete error %s: %v", docID, err)
				} else {
					log.Printf("[vector-watcher] deleted: %s/%s", scope, docID)
				}
			}
		}
	}
}

func (w *VectorWatcher) scopeForPath(path string) string {
	for dir, scope := range w.dirs {
		if strings.HasPrefix(path, dir) {
			return scope
		}
	}
	return ""
}

// Watcher is the global VectorWatcher instance (set by StartVectorWatcher).
var Watcher *VectorWatcher

// SyncFileToVector is called externally (e.g., after WriteFile API) to trigger immediate sync.
// Also registers the file's parent directory for watching if it's a new team scope dir.
// sessionID is the hub session that wrote the file (0 if unknown / watcher-originated).
func SyncFileToVector(scope, filePath string, sessionID int64) {
	// Dynamically register the parent dir for watching (covers new team scope dirs)
	if Watcher != nil {
		dir := filepath.Dir(filePath)
		Watcher.AddWatchDir(dir, scope)
	}
	if Vector == nil || !Vector.IsReady() {
		return
	}
	syncFileToVector(scope, filePath, sessionID)
	// After embedding, update the watcher snapshot so the next poll() cycle won't
	// treat this file as "changed" and re-embed with sessionID=0, overwriting the
	// valid sessionID that was just stored.
	if Watcher != nil {
		if info, err := os.Stat(filePath); err == nil {
			Watcher.mu.Lock()
			Watcher.snapshots[filePath] = fileSnapshot{modTime: info.ModTime(), size: info.Size()}
			Watcher.mu.Unlock()
		}
	}
}

// extractSessionID returns the source_session_id from existing vector metadata for a doc.
// Returns 0 if not found or <= 0. JSON numbers are decoded as float64 by Go's json package.
func extractSessionID(scopeMeta map[string]map[string]interface{}, docID string) int64 {
	if scopeMeta == nil {
		return 0
	}
	docMeta, ok := scopeMeta[docID]
	if !ok {
		return 0
	}
	v, ok := docMeta["source_session_id"]
	if !ok {
		return 0
	}
	if id, ok := v.(float64); ok && id > 0 {
		return int64(id)
	}
	return 0
}

func syncFileToVector(scope, path string, sessionID int64) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[vector-watcher] read error %s: %v", path, err)
		return
	}
	content := string(data)
	name := filepath.Base(path)

	// 向量化内容：文件名 + 内容前200字符
	text := name + "\n"
	if len(content) > 200 {
		text += content[:200]
	} else {
		text += content
	}

	docID := name
	meta := map[string]interface{}{
		"file_path":         path,
		"scope":             scope,
		"source_session_id": sessionID,
	}
	if err := Vector.Embed(scope, docID, text, meta); err != nil {
		log.Printf("[vector-watcher] embed error %s: %v", docID, err)
	} else {
		log.Printf("[vector-watcher] synced: %s/%s (session=%d)", scope, docID, sessionID)
	}
}
