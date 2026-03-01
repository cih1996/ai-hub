package core

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// VectorWatcher polls knowledge/ and memory/ directories for changes
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
// Also discovers existing team-level knowledge/memory dirs under ~/.ai-hub/<groupname>/.
func StartVectorWatcher() *VectorWatcher {
	home, _ := os.UserHomeDir()
	dirs := map[string]string{
		filepath.Join(home, ".ai-hub", "knowledge"): "knowledge",
		filepath.Join(home, ".ai-hub", "memory"):    "memory",
	}
	// Discover existing team-level knowledge/memory directories
	aiHubDir := filepath.Join(home, ".ai-hub")
	builtinDirs := map[string]bool{
		"knowledge": true, "memory": true, "rules": true,
		"skills": true, "notes": true, "scripts": true,
		"vector-engine": true, "logs": true,
	}
	if entries, err := os.ReadDir(aiHubDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || builtinDirs[e.Name()] {
				continue
			}
			for _, sub := range []string{"knowledge", "memory"} {
				subDir := filepath.Join(aiHubDir, e.Name(), sub)
				if info, err2 := os.Stat(subDir); err2 == nil && info.IsDir() {
					scope := e.Name() + "/" + sub
					dirs[subDir] = scope
					log.Printf("[vector-watcher] discovered team dir: %s -> scope=%s", subDir, scope)
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

	for dir, scope := range w.dirs {
		os.MkdirAll(dir, 0755)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}
			w.snapshots[path] = fileSnapshot{modTime: info.ModTime(), size: info.Size()}
			syncFileToVector(scope, path)
		}
	}
	log.Printf("[vector-watcher] initial sync: %d files", len(w.snapshots))
}

func (w *VectorWatcher) poll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	currentFiles := make(map[string]bool)

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
				syncFileToVector(scope, path)
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
func SyncFileToVector(scope, filePath string) {
	// Dynamically register the parent dir for watching (covers new team scope dirs)
	if Watcher != nil {
		dir := filepath.Dir(filePath)
		Watcher.AddWatchDir(dir, scope)
	}
	if Vector == nil || !Vector.IsReady() {
		return
	}
	syncFileToVector(scope, filePath)
}

func syncFileToVector(scope, path string) {
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
		"file_path": path,
		"scope":     scope,
	}
	if err := Vector.Embed(scope, docID, text, meta); err != nil {
		log.Printf("[vector-watcher] embed error %s: %v", docID, err)
	} else {
		log.Printf("[vector-watcher] synced: %s/%s", scope, docID)
	}
}
