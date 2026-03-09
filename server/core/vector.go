package core

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nlpodyssey/cybertron/pkg/models/bert"
	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
)

const (
	// DefaultModelName is the Chinese embedding model
	DefaultModelName = "BAAI/bge-small-zh-v1.5"
	// VectorDimension is the output dimension of the model
	VectorDimension = 512
)

// VectorRecord represents a stored vector with metadata
type VectorRecord struct {
	ID         string                 `json:"id"`
	Document   string                 `json:"document"`
	Vector     []float64              `json:"vector"`
	Metadata   map[string]interface{} `json:"metadata"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
	HitCount   int                    `json:"hit_count"`
	LastHitAt  string                 `json:"last_hit_at"`
}

// VectorEngine manages the Go-native vector engine
type VectorEngine struct {
	mu       sync.RWMutex
	ready    bool
	disabled bool
	err      string

	// model
	model    textencoding.Interface
	modelDir string

	// storage: scope -> (docID -> record)
	collections map[string]map[string]*VectorRecord
	dataDir     string
}

var Vector *VectorEngine

// InitVectorEngine initializes the vector engine
func InitVectorEngine(_ string) {
	baseDir := GetDataDir()
	modelDir := filepath.Join(baseDir, "models")
	dataDir := filepath.Join(baseDir, "vector-data")

	Vector = &VectorEngine{
		modelDir:    modelDir,
		dataDir:     dataDir,
		collections: make(map[string]map[string]*VectorRecord),
	}

	go Vector.bootstrap()
}

func (v *VectorEngine) bootstrap() {
	log.Println("[vector] starting bootstrap...")

	// Step 1: Load embedding model
	if err := v.loadModel(); err != nil {
		v.setDisabled("model load failed: " + err.Error())
		return
	}
	log.Println("[vector] embedding model loaded")

	// Step 2: Load existing data
	if err := v.loadData(); err != nil {
		log.Printf("[vector] warning: failed to load existing data: %v", err)
	}

	v.mu.Lock()
	v.ready = true
	v.mu.Unlock()
	log.Println("[vector] engine ready")
}

func (v *VectorEngine) loadModel() error {
	os.MkdirAll(v.modelDir, 0755)

	// Check if model exists locally
	modelPath := filepath.Join(v.modelDir, strings.ReplaceAll(DefaultModelName, "/", string(os.PathSeparator)))
	spagoModel := filepath.Join(modelPath, "spago_model.bin")

	var downloadPolicy tasks.DownloadPolicy
	if _, err := os.Stat(spagoModel); err == nil {
		// Model exists, use offline mode
		downloadPolicy = tasks.DownloadNever
		log.Printf("[vector] using cached model: %s", modelPath)
	} else {
		// Model doesn't exist, need to download
		downloadPolicy = tasks.DownloadMissing
		log.Printf("[vector] downloading model: %s", DefaultModelName)
	}

	model, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir:      v.modelDir,
		ModelName:      DefaultModelName,
		DownloadPolicy: downloadPolicy,
	})
	if err != nil {
		return fmt.Errorf("failed to load model: %w", err)
	}

	v.model = model
	return nil
}

func (v *VectorEngine) loadData() error {
	os.MkdirAll(v.dataDir, 0755)

	entries, err := os.ReadDir(v.dataDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			scope := strings.TrimSuffix(entry.Name(), ".json")
			if err := v.loadCollection(scope); err != nil {
				log.Printf("[vector] failed to load collection %s: %v", scope, err)
			}
		}
	}
	return nil
}

func (v *VectorEngine) loadCollection(scope string) error {
	filePath := filepath.Join(v.dataDir, scope+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var records map[string]*VectorRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}

	v.mu.Lock()
	v.collections[scope] = records
	v.mu.Unlock()

	log.Printf("[vector] loaded collection %s with %d records", scope, len(records))
	return nil
}

func (v *VectorEngine) saveCollection(scope string) error {
	v.mu.RLock()
	records := v.collections[scope]
	v.mu.RUnlock()

	if records == nil {
		return nil
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(v.dataDir, scope+".json")
	return os.WriteFile(filePath, data, 0644)
}

// scopeToFileName converts scope to safe filename
func scopeToFileName(scope string) string {
	if scope == "memory" {
		return "memory"
	}
	// Team scope: "TeamName/memory" -> "team_memory_<hash>"
	parts := strings.SplitN(scope, "/", 2)
	if len(parts) == 2 {
		h := md5.Sum([]byte(scope))
		return fmt.Sprintf("team_%s_%s", parts[1], hex.EncodeToString(h[:6]))
	}
	return scope
}

func (v *VectorEngine) encode(text string) ([]float64, error) {
	result, err := v.model.Encode(context.Background(), text, int(bert.MeanPooling))
	if err != nil {
		return nil, err
	}
	return result.Vector.Data().F64(), nil
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// Stop shuts down the vector engine
func (v *VectorEngine) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ready = false
	log.Println("[vector] stopped")
}

// Restart restarts the vector engine
func (v *VectorEngine) Restart() {
	log.Println("[vector] restart requested")
	v.Stop()

	v.mu.Lock()
	v.disabled = false
	v.err = ""
	v.mu.Unlock()

	v.bootstrap()
}

// IsReady returns whether the vector engine is available
func (v *VectorEngine) IsReady() bool {
	if v == nil {
		return false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.ready
}

// IsDisabled returns whether the vector engine is permanently disabled
func (v *VectorEngine) IsDisabled() bool {
	if v == nil {
		return true
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.disabled
}

// WaitReady blocks until the engine is ready, disabled, or timeout
func (v *VectorEngine) WaitReady(timeout time.Duration) bool {
	if v == nil {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if v.IsReady() {
			return true
		}
		if v.IsDisabled() {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
	return v.IsReady()
}

// Status returns current engine status
func (v *VectorEngine) Status() map[string]interface{} {
	if v == nil {
		return map[string]interface{}{"ready": false, "disabled": true, "error": "not initialized"}
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	return map[string]interface{}{
		"ready":    v.ready,
		"disabled": v.disabled,
		"error":    v.err,
		"model":    DefaultModelName,
	}
}

func (v *VectorEngine) setDisabled(reason string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.disabled = true
	v.err = reason
	log.Printf("[vector] disabled: %s", reason)
}

// Embed adds or updates a vector record
func (v *VectorEngine) Embed(scope, docID, text string, metadata map[string]interface{}) error {
	if !v.IsReady() {
		return fmt.Errorf("vector engine not ready")
	}

	vector, err := v.encode(text)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	now := time.Now().Format("2006-01-02T15:04:05")
	fileName := scopeToFileName(scope)

	v.mu.Lock()
	if v.collections[fileName] == nil {
		v.collections[fileName] = make(map[string]*VectorRecord)
	}

	existing := v.collections[fileName][docID]
	record := &VectorRecord{
		ID:        docID,
		Document:  text,
		Vector:    vector,
		Metadata:  metadata,
		UpdatedAt: now,
	}

	if existing != nil {
		record.CreatedAt = existing.CreatedAt
		record.HitCount = existing.HitCount
		record.LastHitAt = existing.LastHitAt
	} else {
		record.CreatedAt = now
	}

	if record.Metadata == nil {
		record.Metadata = make(map[string]interface{})
	}
	record.Metadata["created_at"] = record.CreatedAt
	record.Metadata["updated_at"] = record.UpdatedAt
	record.Metadata["hit_count"] = record.HitCount
	record.Metadata["last_hit_time"] = record.LastHitAt

	v.collections[fileName][docID] = record
	v.mu.Unlock()

	return v.saveCollection(fileName)
}

// UpdateMetadata merges updates into existing metadata
func (v *VectorEngine) UpdateMetadata(scope, docID string, updates map[string]interface{}) (map[string]interface{}, error) {
	if !v.IsReady() {
		return nil, fmt.Errorf("vector engine not ready")
	}

	fileName := scopeToFileName(scope)

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.collections[fileName] == nil {
		return nil, fmt.Errorf("doc not found: %s", docID)
	}

	record := v.collections[fileName][docID]
	if record == nil {
		return nil, fmt.Errorf("doc not found: %s", docID)
	}

	if record.Metadata == nil {
		record.Metadata = make(map[string]interface{})
	}
	for k, val := range updates {
		record.Metadata[k] = val
	}
	record.UpdatedAt = time.Now().Format("2006-01-02T15:04:05")
	record.Metadata["updated_at"] = record.UpdatedAt

	go v.saveCollection(fileName)
	return record.Metadata, nil
}

// GetDoc retrieves a single document
func (v *VectorEngine) GetDoc(scope, docID string) (map[string]interface{}, error) {
	if !v.IsReady() {
		return nil, fmt.Errorf("vector engine not ready")
	}

	fileName := scopeToFileName(scope)

	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.collections[fileName] == nil {
		return nil, fmt.Errorf("doc not found: %s", docID)
	}

	record := v.collections[fileName][docID]
	if record == nil {
		return nil, fmt.Errorf("doc not found: %s", docID)
	}

	return map[string]interface{}{
		"id":       record.ID,
		"document": record.Document,
		"metadata": record.Metadata,
	}, nil
}

// Search performs semantic search
func (v *VectorEngine) Search(scope, query string, topK int) ([]map[string]interface{}, error) {
	if !v.IsReady() {
		return nil, fmt.Errorf("vector engine not ready")
	}

	queryVector, err := v.encode(query)
	if err != nil {
		return nil, fmt.Errorf("encode query failed: %w", err)
	}

	fileName := scopeToFileName(scope)

	v.mu.RLock()
	records := v.collections[fileName]
	v.mu.RUnlock()

	if records == nil || len(records) == 0 {
		return []map[string]interface{}{}, nil
	}

	// Calculate similarities
	type scored struct {
		record     *VectorRecord
		similarity float64
	}
	var results []scored

	for _, record := range records {
		sim := cosineSimilarity(queryVector, record.Vector)
		results = append(results, scored{record: record, similarity: sim})
	}

	// Sort by similarity (descending)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].similarity > results[i].similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Take top K
	if topK > len(results) {
		topK = len(results)
	}
	results = results[:topK]

	// Record hits and format output
	items := make([]map[string]interface{}, 0, len(results))
	now := time.Now().Format("2006-01-02T15:04:05")

	v.mu.Lock()
	for _, r := range results {
		r.record.HitCount++
		r.record.LastHitAt = now
		if r.record.Metadata != nil {
			r.record.Metadata["hit_count"] = r.record.HitCount
			r.record.Metadata["last_hit_time"] = r.record.LastHitAt
		}

		items = append(items, map[string]interface{}{
			"id":         r.record.ID,
			"document":   r.record.Document,
			"similarity": r.similarity,
			"metadata":   r.record.Metadata,
		})
	}
	v.mu.Unlock()

	go v.saveCollection(fileName)

	return items, nil
}

// Delete removes a vector record
func (v *VectorEngine) Delete(scope, docID string) error {
	if !v.IsReady() {
		return fmt.Errorf("vector engine not ready")
	}

	fileName := scopeToFileName(scope)

	v.mu.Lock()
	if v.collections[fileName] != nil {
		delete(v.collections[fileName], docID)
	}
	v.mu.Unlock()

	return v.saveCollection(fileName)
}

// ListMetadata returns all records' metadata for a scope
func (v *VectorEngine) ListMetadata(scope string) map[string]map[string]interface{} {
	if !v.IsReady() {
		return nil
	}

	fileName := scopeToFileName(scope)

	v.mu.RLock()
	defer v.mu.RUnlock()

	records := v.collections[fileName]
	if records == nil {
		return nil
	}

	result := make(map[string]map[string]interface{})
	for id, record := range records {
		result[id] = record.Metadata
	}
	return result
}

// Stats returns hit statistics
func (v *VectorEngine) Stats(scope string) (map[string]interface{}, error) {
	if !v.IsReady() {
		return nil, fmt.Errorf("vector engine not ready")
	}

	fileName := scopeToFileName(scope)

	v.mu.RLock()
	defer v.mu.RUnlock()

	records := v.collections[fileName]
	if records == nil {
		return map[string]interface{}{"total": 0, "records": []interface{}{}}, nil
	}

	var statRecords []map[string]interface{}
	for _, record := range records {
		statRecords = append(statRecords, map[string]interface{}{
			"id":            record.ID,
			"hit_count":     record.HitCount,
			"last_hit_time": record.LastHitAt,
		})
	}

	// Sort by hit_count descending
	for i := 0; i < len(statRecords)-1; i++ {
		for j := i + 1; j < len(statRecords); j++ {
			if statRecords[j]["hit_count"].(int) > statRecords[i]["hit_count"].(int) {
				statRecords[i], statRecords[j] = statRecords[j], statRecords[i]
			}
		}
	}

	return map[string]interface{}{
		"total":   len(records),
		"records": statRecords,
	}, nil
}
