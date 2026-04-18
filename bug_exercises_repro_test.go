package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func withTempRuntimeDirs(t *testing.T, fn func()) {
	t.Helper()

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("bug-exercises-%d", time.Now().UnixNano()))
	origBaseDir := baseDir
	origImagesDir := imagesDir
	origLayersDir := layersDir

	baseDir = tmp
	imagesDir = filepath.Join(baseDir, "images")
	layersDir = filepath.Join(baseDir, "layers")

	if err := os.MkdirAll(filepath.Join(baseDir, "containers"), 0o755); err != nil {
		t.Fatalf("failed to create temp container dir: %v", err)
	}
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("failed to create temp images dir: %v", err)
	}
	if err := os.MkdirAll(layersDir, 0o755); err != nil {
		t.Fatalf("failed to create temp layers dir: %v", err)
	}

	defer func() {
		baseDir = origBaseDir
		imagesDir = origImagesDir
		layersDir = origLayersDir
		_ = os.RemoveAll(tmp)
	}()

	fn()
}

func captureStdoutForTest(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func mustSaveStateForRepro(t *testing.T, metadata ContainerMetadata) {
	t.Helper()
	if err := SaveContainerState(metadata); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}
}

func TestBugExercise1_ReproduceShortContainerIDPanic(t *testing.T) {
	withTempRuntimeDirs(t, func() {
		containerID := "abc"
		containerDir := filepath.Join(baseDir, "containers", containerID)
		if err := os.MkdirAll(containerDir, 0o755); err != nil {
			t.Fatalf("failed to create container dir: %v", err)
		}

		cm := NewContainerMonitor(containerID)
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("expected panic for short container ID, got none")
			}
		}()

		_, _ = cm.GetMetrics()
	})
}

func TestBugExercise2_ReproduceNilDeferClosePanic(t *testing.T) {
	src, err := os.ReadFile("/home/runner/work/basic-docker-engine/basic-docker-engine/main.go")
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}
	content := string(src)

	start := strings.Index(content, "func saveLayerMetadata(layer ImageLayer) error {")
	if start == -1 {
		t.Fatalf("saveLayerMetadata function not found")
	}
	end := strings.Index(content[start:], "func mountLayeredFilesystem(")
	if end == -1 {
		t.Fatalf("saveLayerMetadata function boundary not found")
	}
	section := content[start : start+end]

	deferIdx := strings.Index(section, "defer file.Close()")
	errCheckIdx := strings.Index(section, "if err != nil")
	if deferIdx == -1 || errCheckIdx == -1 {
		t.Fatalf("expected defer and error-check patterns in saveLayerMetadata")
	}
	if deferIdx > errCheckIdx {
		t.Fatalf("expected reproduction: defer file.Close appears before os.Create error check")
	}
}

func TestBugExercise3_ReproduceWrongPIDTracked(t *testing.T) {
	withTempRuntimeDirs(t, func() {
		origCgroup := hasCgroupAccess
		hasCgroupAccess = false
		defer func() { hasCgroupAccess = origCgroup }()

		containerID := "bug3-container"
		mustSaveStateForRepro(t, ContainerMetadata{
			ID:        containerID,
			State:     StateCreated,
			Image:     "test",
			Command:   "/bin/sh",
			CreatedAt: time.Now(),
		})

		runWithoutNamespaces(containerID, "", "/bin/sh", []string{"-c", "sleep 0.1"})

		updated, err := LoadContainerState(containerID)
		if err != nil {
			t.Fatalf("failed to load updated state: %v", err)
		}

		if updated.PID != os.Getpid() {
			t.Fatalf("expected reproduced bug PID=%d (caller PID), got %d", os.Getpid(), updated.PID)
		}
	})
}

func TestBugExercise4_ReproduceMissingPIDFileContract(t *testing.T) {
	withTempRuntimeDirs(t, func() {
		origCgroup := hasCgroupAccess
		hasCgroupAccess = false
		defer func() { hasCgroupAccess = origCgroup }()

		containerID := "bug4-container"
		mustSaveStateForRepro(t, ContainerMetadata{
			ID:        containerID,
			State:     StateCreated,
			Image:     "test",
			Command:   "/bin/sh",
			CreatedAt: time.Now(),
		})

		runWithoutNamespaces(containerID, "", "/bin/sh", []string{"-c", "sleep 0.1"})

		pidFile := filepath.Join(baseDir, "containers", containerID, "pid")
		if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
			t.Fatalf("expected no pid file to reproduce ps/exec contract bug, stat err=%v", err)
		}
	})
}

type trackingLayerReader struct {
	io.Reader
	onClose func()
}

func (r *trackingLayerReader) Close() error {
	if r.onClose != nil {
		r.onClose()
	}
	return nil
}

type reproRegistry struct {
	manifest    Manifest
	layers      map[string][]byte
	mu          sync.Mutex
	currentOpen int
	maxOpen     int
}

func (r *reproRegistry) FetchManifest(repo, tag string) (*Manifest, error) {
	return &r.manifest, nil
}

func (r *reproRegistry) FetchLayer(repo, digest string) (io.ReadCloser, error) {
	data, ok := r.layers[digest]
	if !ok {
		return nil, fmt.Errorf("missing digest %s", digest)
	}

	r.mu.Lock()
	r.currentOpen++
	if r.currentOpen > r.maxOpen {
		r.maxOpen = r.currentOpen
	}
	r.mu.Unlock()

	return &trackingLayerReader{
		Reader: bytes.NewReader(data),
		onClose: func() {
			r.mu.Lock()
			r.currentOpen--
			r.mu.Unlock()
		},
	}, nil
}

func makeTarLayerForRepro(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	return buf.Bytes()
}

func TestBugExercise5_ReproduceDeferredCloseInLoop(t *testing.T) {
	uniqueImage := fmt.Sprintf("bug5-image-%d", time.Now().UnixNano())
	_ = os.RemoveAll(filepath.Join("/tmp/basic-docker/images", uniqueImage))
	defer os.RemoveAll(filepath.Join("/tmp/basic-docker/images", uniqueImage))

	r := &reproRegistry{
		layers: map[string][]byte{},
	}
	digests := []string{"sha256:l1", "sha256:l2", "sha256:l3"}
	for i, d := range digests {
		r.manifest.Layers = append(r.manifest.Layers, struct {
			Digest string "json:\"digest\""
		}{Digest: d})
		r.layers[d] = makeTarLayerForRepro(t, fmt.Sprintf("f-%d.txt", i), "x")
	}

	if _, err := Pull(r, uniqueImage); err != nil {
		t.Fatalf("pull failed during repro: %v", err)
	}

	r.mu.Lock()
	maxOpen := r.maxOpen
	r.mu.Unlock()

	if maxOpen <= 1 {
		t.Fatalf("expected >1 simultaneously open layer readers from defer-in-loop bug, got %d", maxOpen)
	}
}

func TestBugExercise6_ReproduceHardcodedImageDirectoryMismatch(t *testing.T) {
	withTempRuntimeDirs(t, func() {
		uniqueImage := fmt.Sprintf("bug6-image-%d", time.Now().UnixNano())
		if err := os.MkdirAll(filepath.Join(imagesDir, uniqueImage, "rootfs"), 0o755); err != nil {
			t.Fatalf("failed to create image in configured imagesDir: %v", err)
		}

		outputA := captureStdoutForTest(t, ListImages)
		if strings.Contains(outputA, uniqueImage) {
			t.Fatalf("expected ListImages to ignore configured imagesDir and miss image %s", uniqueImage)
		}

		outputB := captureStdoutForTest(t, listImages)
		if strings.Contains(outputB, uniqueImage) {
			t.Fatalf("expected listImages to ignore configured imagesDir and miss image %s", uniqueImage)
		}
	})
}

func TestBugExercise7_ReproduceUnsafeExtractionLackOfValidation(t *testing.T) {
	src, err := os.ReadFile("/home/runner/work/basic-docker-engine/basic-docker-engine/image.go")
	if err != nil {
		t.Fatalf("failed to read image.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `exec.Command("tar", "-x", "-C", rootfs)`) {
		t.Fatalf("expected tar extraction command pattern in extractLayer")
	}
	if strings.Contains(content, "filepath.Clean") || strings.Contains(content, "Rel(") {
		t.Fatalf("expected no explicit path-boundary validation in extractLayer/LoadImageFromTar")
	}
}

func TestBugExercise8_ReproduceIgnoredStateUpdateErrors(t *testing.T) {
	withTempRuntimeDirs(t, func() {
		origCgroup := hasCgroupAccess
		hasCgroupAccess = false
		defer func() { hasCgroupAccess = origCgroup }()

		containerID := "bug8-container"
		mustSaveStateForRepro(t, ContainerMetadata{
			ID:        containerID,
			State:     StateCreated,
			Image:     "test",
			Command:   "/bin/true",
			CreatedAt: time.Now(),
		})

		containerDir := filepath.Join(baseDir, "containers", containerID)
		if err := os.Chmod(containerDir, 0o555); err != nil {
			t.Fatalf("failed to chmod container dir: %v", err)
		}
		defer os.Chmod(containerDir, 0o755)
		stateFile := filepath.Join(containerDir, "state.json")
		if err := os.Chmod(stateFile, 0o444); err != nil {
			t.Fatalf("failed to chmod state file: %v", err)
		}
		defer os.Chmod(stateFile, 0o644)

		runWithoutNamespaces(containerID, "", "/bin/true", nil)

		updated, err := LoadContainerState(containerID)
		if err != nil {
			t.Fatalf("failed to load state after run: %v", err)
		}

		if updated.State != StateCreated {
			t.Fatalf("expected state to remain %q because update errors are ignored, got %q", StateCreated, updated.State)
		}
	})
}

func TestBugExercise9_ReproduceGlobalMutableNetworkStateDesign(t *testing.T) {
	src, err := os.ReadFile("/home/runner/work/basic-docker-engine/basic-docker-engine/network.go")
	if err != nil {
		t.Fatalf("failed to read network.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "var networks = []Network{}") {
		t.Fatalf("expected global mutable networks state declaration")
	}
	if strings.Contains(content, "sync.Mutex") || strings.Contains(content, "sync.RWMutex") {
		t.Fatalf("expected reproduction state: no synchronization around global networks state")
	}
}
