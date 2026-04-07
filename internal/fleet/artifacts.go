package fleet

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

func (c *Coordinator) artifactPath(workID string) string {
	if c.artifactDir == "" {
		return ""
	}
	return filepath.Join(c.artifactDir, workID, "result.bundle")
}

func (c *Coordinator) handleWorkArtifactUpload(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("workID")
	if workID == "" {
		http.Error(w, "work id required", http.StatusBadRequest)
		return
	}
	item, ok := c.queue.Get(workID)
	if !ok {
		http.Error(w, "work item not found", http.StatusNotFound)
		return
	}
	if item.Source != WorkSourceStructuredCodexTeam {
		http.Error(w, "artifacts are only supported for structured codex work items", http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, fmt.Sprintf("parse multipart: %v", err), http.StatusBadRequest)
		return
	}
	metaField := r.FormValue("metadata")
	if metaField == "" {
		http.Error(w, "metadata is required", http.StatusBadRequest)
		return
	}
	var meta ArtifactUploadMetadata
	if err := json.Unmarshal([]byte(metaField), &meta); err != nil {
		http.Error(w, fmt.Sprintf("parse metadata: %v", err), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("artifact")
	if err != nil {
		http.Error(w, fmt.Sprintf("artifact file required: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	target := c.artifactPath(workID)
	if target == "" {
		http.Error(w, "artifact storage is not configured", http.StatusServiceUnavailable)
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		http.Error(w, fmt.Sprintf("create artifact dir: %v", err), http.StatusInternalServerError)
		return
	}

	if err := writeUploadedArtifact(target, file); err != nil {
		http.Error(w, fmt.Sprintf("store artifact: %v", err), http.StatusInternalServerError)
		return
	}
	hash, size, err := hashFile(target)
	if err != nil {
		http.Error(w, fmt.Sprintf("verify artifact: %v", err), http.StatusInternalServerError)
		return
	}
	if meta.ArtifactHash != "" && meta.ArtifactHash != hash {
		_ = os.Remove(target)
		http.Error(w, "artifact hash mismatch", http.StatusBadRequest)
		return
	}
	if meta.ArtifactSizeBytes > 0 && meta.ArtifactSizeBytes != size {
		_ = os.Remove(target)
		http.Error(w, "artifact size mismatch", http.StatusBadRequest)
		return
	}

	if item.Result == nil {
		item.Result = &WorkResult{}
	}
	item.Result.ArtifactType = meta.ArtifactType
	item.Result.ArtifactPath = target
	item.Result.ArtifactHash = hash
	item.Result.ArtifactSizeBytes = size
	item.Result.ArtifactBaseRef = meta.ArtifactBaseRef
	item.Result.ArtifactTipRef = meta.ArtifactTipRef
	item.Result.ArtifactStatus = "uploaded"
	c.queue.Update(item)

	writeJSON(w, map[string]any{
		"artifact_path":       target,
		"artifact_hash":       hash,
		"artifact_size_bytes": size,
		"artifact_status":     "uploaded",
	})
}

func (c *Coordinator) handleWorkArtifactGet(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("workID")
	if workID == "" {
		http.Error(w, "work id required", http.StatusBadRequest)
		return
	}
	item, ok := c.queue.Lookup(workID)
	if !ok || item.Result == nil || item.Result.ArtifactPath == "" {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{
		"work_item_id":        workID,
		"artifact_type":       item.Result.ArtifactType,
		"artifact_path":       item.Result.ArtifactPath,
		"artifact_hash":       item.Result.ArtifactHash,
		"artifact_size_bytes": item.Result.ArtifactSizeBytes,
		"artifact_base_ref":   item.Result.ArtifactBaseRef,
		"artifact_tip_ref":    item.Result.ArtifactTipRef,
		"artifact_status":     item.Result.ArtifactStatus,
	})
}

func writeUploadedArtifact(path string, file multipart.File) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	return err
}
