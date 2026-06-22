package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Andrew0613/Anton/internal/adapter"
)

const ManifestFilename = "run.json"
const manifestLockPollInterval = 25 * time.Millisecond
const manifestLockTimeout = 10 * time.Second

var manifestMutexes sync.Map

type Store struct {
	bundleRoot  string
	path        string
	receiptsDir string
}

func NewStore(bundleRoot string) Store {
	return NewStoreWithNames(bundleRoot, ManifestFilename, "receipts")
}

func NewStoreWithNames(bundleRoot string, manifestName string, receiptsDir string) Store {
	cleanRoot := filepath.Clean(bundleRoot)
	manifestName = strings.TrimSpace(manifestName)
	receiptsDir = strings.TrimSpace(receiptsDir)
	if manifestName == "" {
		manifestName = ManifestFilename
	}
	if receiptsDir == "" {
		receiptsDir = "receipts"
	}
	return Store{
		bundleRoot:  cleanRoot,
		path:        filepath.Join(cleanRoot, manifestName),
		receiptsDir: filepath.Join(cleanRoot, receiptsDir),
	}
}

func ResolveStore(workingDirectory string, environ []string, now time.Time) (Store, adapter.ResolvedTaskBundle, adapter.Resolved, error) {
	resolved, err := adapter.Resolve(workingDirectory, environ)
	if err != nil {
		return Store{}, adapter.ResolvedTaskBundle{}, adapter.Resolved{}, err
	}
	bundle, err := resolved.Definition.TaskBundle(resolved.Context, environ, now)
	if err != nil {
		return Store{}, adapter.ResolvedTaskBundle{}, resolved, err
	}
	return NewStoreWithNames(bundle.Root, resolved.Config.RunManifestName(), resolved.Config.RunReceiptsDir()), bundle, resolved, nil
}

func (store Store) BundleRoot() string {
	return store.bundleRoot
}

func (store Store) Path() string {
	return store.path
}

func (store Store) ReceiptsDir() string {
	return store.receiptsDir
}

func (store Store) WriteReceipt(kind string, name string, content []byte) (string, error) {
	if err := store.requireExistingBundle(); err != nil {
		return "", err
	}
	if err := store.validateSidecarPaths(); err != nil {
		return "", err
	}
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if kind == "" {
		return "", fmt.Errorf("receipt kind is required")
	}
	if name == "" {
		return "", fmt.Errorf("receipt name is required")
	}
	if strings.ContainsAny(kind, `/\`) {
		return "", fmt.Errorf("receipt kind must be one path segment: %s", kind)
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("receipt name must be one path segment: %s", name)
	}
	if err := ensureSafeDir(store.receiptsDir, "run receipts dir"); err != nil {
		return "", err
	}
	receiptDir := filepath.Join(store.receiptsDir, kind)
	if err := ensureSafeDir(receiptDir, "receipt kind dir"); err != nil {
		return "", err
	}
	receiptPath := filepath.Join(receiptDir, name)
	if err := rejectSymlinkFile(receiptPath, "receipt path"); err != nil {
		return "", err
	}
	if err := os.WriteFile(receiptPath, content, 0o644); err != nil {
		return "", err
	}
	return receiptPath, nil
}

func (store Store) Exists() bool {
	info, err := os.Stat(store.path)
	return err == nil && !info.IsDir()
}

func (store Store) Load() (Manifest, error) {
	if err := store.requireExistingBundle(); err != nil {
		return Manifest{}, err
	}
	if err := store.validateSidecarPaths(); err != nil {
		return Manifest{}, err
	}
	content, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, fmt.Errorf("run manifest is missing at %s; run `anton run init` after `anton task-state init`", store.path)
		}
		return Manifest{}, fmt.Errorf("read %s: %w", store.path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", store.path, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate %s: %w", store.path, err)
	}
	return manifest, nil
}

func (store Store) LoadForTask(taskID string) (Manifest, error) {
	manifest, err := store.Load()
	if err != nil {
		return Manifest{}, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Manifest{}, fmt.Errorf("expected task id is required")
	}
	if manifest.TaskID != taskID {
		return Manifest{}, fmt.Errorf("run manifest task_id %q does not match active task %q", manifest.TaskID, taskID)
	}
	return manifest, nil
}

func (store Store) Init(taskID string, now time.Time) (Manifest, error) {
	if err := store.requireExistingBundle(); err != nil {
		return Manifest{}, err
	}
	if err := store.validateSidecarPaths(); err != nil {
		return Manifest{}, err
	}
	lock, err := store.acquireManifestLock()
	if err != nil {
		return Manifest{}, err
	}
	defer lock.Release()
	if store.Exists() {
		return store.LoadForTask(taskID)
	}
	manifest, err := NewManifest(taskID, now)
	if err != nil {
		return Manifest{}, err
	}
	if err := store.Save(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (store Store) UpdateForTask(taskID string, now time.Time, mutate func(*Manifest) error) (Manifest, error) {
	if err := store.requireExistingBundle(); err != nil {
		return Manifest{}, err
	}
	if err := store.validateSidecarPaths(); err != nil {
		return Manifest{}, err
	}
	lock, err := store.acquireManifestLock()
	if err != nil {
		return Manifest{}, err
	}
	defer lock.Release()

	manifest, err := store.LoadForTask(taskID)
	if err != nil {
		return Manifest{}, err
	}
	if mutate == nil {
		return Manifest{}, fmt.Errorf("run manifest mutation is required")
	}
	if err := mutate(&manifest); err != nil {
		return Manifest{}, err
	}
	if err := store.Save(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (store Store) Save(manifest Manifest) error {
	if err := store.requireExistingBundle(); err != nil {
		return err
	}
	if err := store.validateSidecarPaths(); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := store.rejectSymlinkPath(); err != nil {
		return err
	}

	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	content = append(content, '\n')

	tmp, err := os.CreateTemp(store.bundleRoot, "."+ManifestFilename+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp manifest in %s: %w", store.bundleRoot, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp manifest %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp manifest %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp manifest %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, store.path); err != nil {
		return fmt.Errorf("replace manifest %s: %w", store.path, err)
	}
	return nil
}

type manifestLock struct {
	file  *os.File
	mutex *sync.Mutex
}

func (store Store) acquireManifestLock() (*manifestLock, error) {
	mutex := store.localManifestMutex()
	mutex.Lock()

	file, err := os.Open(store.bundleRoot)
	if err != nil {
		mutex.Unlock()
		return nil, fmt.Errorf("open task bundle root for run manifest lock: %w", err)
	}
	deadline := time.Now().Add(manifestLockTimeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &manifestLock{file: file, mutex: mutex}, nil
		}
		if !isLockBusy(err) {
			_ = file.Close()
			mutex.Unlock()
			return nil, fmt.Errorf("lock run manifest bundle %s: %w", store.bundleRoot, err)
		}
		if time.Now().After(deadline) {
			_ = file.Close()
			mutex.Unlock()
			return nil, fmt.Errorf("timed out waiting for run manifest lock: %s", store.bundleRoot)
		}
		time.Sleep(manifestLockPollInterval)
	}
}

func (store Store) localManifestMutex() *sync.Mutex {
	key := filepath.Clean(store.bundleRoot)
	value, _ := manifestMutexes.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (lock *manifestLock) Release() {
	if lock == nil {
		return
	}
	if lock.file != nil {
		_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
		_ = lock.file.Close()
	}
	if lock.mutex != nil {
		lock.mutex.Unlock()
	}
}

func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func (store Store) requireExistingBundle() error {
	info, err := os.Stat(store.bundleRoot)
	if err != nil {
		return fmt.Errorf("task bundle root is required before run manifest writes: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("task bundle root is not a directory: %s", store.bundleRoot)
	}
	return nil
}

func (store Store) rejectSymlinkPath() error {
	return rejectSymlinkFile(store.path, "run manifest path")
}

func (store Store) validateSidecarPaths() error {
	if err := validateBundleLocalSegment(store.bundleRoot, store.path, "run manifest"); err != nil {
		return err
	}
	if err := validateBundleLocalSegment(store.bundleRoot, store.receiptsDir, "run receipts dir"); err != nil {
		return err
	}
	return nil
}

func ensureSafeDir(path string, label string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("create %s %s: %w", label, path, err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("lstat %s %s: %w", label, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular task-bundle directory, not a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s must be a directory: %s", label, path)
	}
	return nil
}

func rejectSymlinkFile(path string, label string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lstat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular task-bundle file, not a symlink: %s", label, path)
	}
	return nil
}

func validateBundleLocalSegment(bundleRoot string, path string, label string) error {
	relative, err := filepath.Rel(bundleRoot, path)
	if err != nil {
		return fmt.Errorf("resolve %s path: %w", label, err)
	}
	if relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s path escapes task bundle: %s", label, path)
	}
	if strings.Contains(relative, string(filepath.Separator)) {
		return fmt.Errorf("%s path must be one path segment inside the task bundle: %s", label, filepath.ToSlash(relative))
	}
	return nil
}
