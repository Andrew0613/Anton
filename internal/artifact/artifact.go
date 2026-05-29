package artifact

import (
	"io/fs"
	"os"
	"path/filepath"
)

type Footprint struct {
	Path        string `json:"path"`
	Exists      bool   `json:"exists"`
	FileCount   int    `json:"file_count"`
	TotalBytes  int64  `json:"total_bytes"`
	WalkLimited bool   `json:"walk_limited"`
}

const maxScannedFiles = 10000

func ScanResultFootprint(workspacePath string) Footprint {
	path := filepath.Join(workspacePath, "results")
	footprint := Footprint{Path: path}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return footprint
	}
	footprint.Exists = true
	_ = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return nil
		}
		footprint.FileCount++
		footprint.TotalBytes += fileInfo.Size()
		if footprint.FileCount >= maxScannedFiles {
			footprint.WalkLimited = true
			return fs.SkipAll
		}
		return nil
	})
	return footprint
}
