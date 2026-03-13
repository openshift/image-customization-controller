/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package imagehandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openshift/image-customization-controller/pkg/env"
)

type closer struct {
	io.ReadSeeker
}

func (c closer) Close() error {
	return nil
}

func nopCloser(stream io.ReadSeeker) io.ReadSeekCloser {
	return closer{stream}
}

func TestImageHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/host-xyz-45-uuid", nil)
	if err != nil {
		t.Fatal(err)
	}

	baseURL, _ := url.Parse("http://localhost:8080")

	rr := httptest.NewRecorder()
	imageServer := &imageFileSystem{
		log: zap.New(zap.UseDevMode(true)),
		isoFiles: map[string]*baseIso{
			"host": {baseFileData{filename: "dummyfile.iso", size: 12345}},
		},
		baseURL: baseURL,
		keys: map[string]string{
			"host-xyz-45-uuid": "host-xyz-45.iso",
		},
		images: map[string]*imageFile{
			"host-xyz-45.iso": {
				name:            "host-xyz-45-uuid",
				size:            12345,
				ignitionContent: []byte("asietonarst"),
				imageReader:     nopCloser(strings.NewReader("aiosetnarsetin")),
			},
		},
		mu: &sync.Mutex{},
	}

	handler := http.FileServer(imageServer.FileSystem())
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	expected := `aiosetnarsetin`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestNewImageHandler(t *testing.T) {
	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	ifs := imageFileSystem{
		baseURL:        baseUrl,
		keys:           map[string]string{},
		mu:             &sync.Mutex{},
		images:         map[string]*imageFile{},
		isoFiles:       map[string]*baseIso{},
		initramfsFiles: map[string]*baseInitramfs{},
	}

	iso := newBaseIso("dummyfile.iso")
	iso.size = 123456
	ifs.isoFiles["host"] = iso

	initramfs := newBaseInitramfs("dummyfile.initramfs")
	initramfs.size = 12345
	ifs.initramfsFiles["host"] = initramfs

	url1, err := ifs.ServeImage("test-key-1", "", []byte{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	url2, err := ifs.ServeImage("test-key-2", "", []byte{}, true, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	name2 := url2[22:]
	if ifs.imageFileByName(name2) == nil {
		t.Errorf("can't look up image file \"%s\"", name2)
	}

	url1again, err := ifs.ServeImage("test-key-1", "", []byte{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	if url1again != url1 {
		t.Errorf("inconsistent URLs for same key: %s %s", url1, url1again)
	}

	ifs.RemoveImage("test-key-1")
	url1yetagain, err := ifs.ServeImage("test-key-1", "", []byte{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if url1yetagain == url1 {
		t.Errorf("same URLs returned after removal: %s", url1yetagain)
	}
}

func TestNewImageHandlerStatic(t *testing.T) {
	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	ifs := imageFileSystem{
		baseURL:        baseUrl,
		keys:           map[string]string{},
		mu:             &sync.Mutex{},
		images:         map[string]*imageFile{},
		isoFiles:       map[string]*baseIso{},
		initramfsFiles: map[string]*baseInitramfs{},
	}

	iso := newBaseIso("dummyfile.iso")
	iso.size = 123456
	ifs.isoFiles["host"] = iso

	initramfs := newBaseInitramfs("dummyfile.initramfs")
	initramfs.size = 12345
	ifs.initramfsFiles["host"] = initramfs

	url1, err := ifs.ServeImage("test-name-1.iso", "", []byte{}, false, true)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	url2, err := ifs.ServeImage("test-name-2.initramfs", "", []byte{}, true, true)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	url1again, err := ifs.ServeImage("test-name-1.iso", "", []byte{}, false, true)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	url1Expected := "http://base.test:1234/test-name-1.iso"
	if url1 != url1Expected {
		t.Errorf("unexpected url %s (should be %s)", url1, url1Expected)
	}
	url2Expected := "http://base.test:1234/test-name-2.initramfs"
	if url2 != url2Expected {
		t.Errorf("unexpected url %s (should be %s)", url2, url2Expected)
	}
	if url1again != url1 {
		t.Errorf("inconsistent URLs for same key: %s %s", url1, url1again)
	}
}

func TestImagePattern(t *testing.T) {
	envInputs := &env.EnvInputs{
		DeployISO:    "/config/ipa.iso",
		DeployInitrd: "/config/ipa.initramfs",
	}

	tcs := []struct {
		name     string
		filename string
		arch     string
		kind     imageKind
		error    bool
	}{
		{
			name:     "host iso - exact path match",
			filename: "/config/ipa.iso",
			arch:     "host",
			kind:     imageKindISO,
		},
		{
			name:     "host initramfs - exact path match",
			filename: "/config/ipa.initramfs",
			arch:     "host",
			kind:     imageKindInitramfs,
		},
		{
			name:     "aarch64 iso - same directory as config",
			filename: "/config/ipa_aarch64.iso",
			arch:     "aarch64",
			kind:     imageKindISO,
		},
		{
			name:     "aarch64 initramfs - different directory from config",
			filename: "/images/ipa_aarch64.initramfs",
			arch:     "aarch64",
			kind:     imageKindInitramfs,
		},
		{
			name:     "x86_64 iso - different directory from config",
			filename: "/images/ipa_x86_64.iso",
			arch:     "x86_64",
			kind:     imageKindISO,
		},
		{
			name:     "x86_64 initramfs - relative path",
			filename: "ipa_x86_64.initramfs",
			arch:     "x86_64",
			kind:     imageKindInitramfs,
		},
		{
			name:     "aarch64 iso - period separator",
			filename: "ipa.aarch64.iso",
			arch:     "aarch64",
			kind:     imageKindISO,
		},
		{
			name:     "x86_64 initramfs - period separator",
			filename: "/images/ipa.x86_64.initramfs",
			arch:     "x86_64",
			kind:     imageKindInitramfs,
		},
		{
			name:     "invalid filename - different base name",
			filename: "/images/different-file_x86_64.iso",
			error:    true,
		},
		{
			name:     "invalid filename - different base name with arch pattern",
			filename: "different-file_aarch64.initramfs",
			error:    true,
		},
		{
			name:     "invalid filename - different base name with period separator",
			filename: "different-file.aarch64.initramfs",
			error:    true,
		},
		{
			name:     "invalid filename - no arch pattern",
			filename: "some-other-file.iso",
			error:    true,
		},
	}

	for _, tc := range tcs {
		t.Logf("testing %s", tc.name)
		ii, err := loadOSImage(envInputs, tc.filename)

		if err != nil && !tc.error {
			t.Errorf("got error: %v", err)
			return
		}

		if err == nil && tc.error {
			t.Errorf("expected error but got none")
			return
		}

		if tc.error {
			continue
		}

		if ii.arch != tc.arch {
			t.Errorf("arch: expected %s but got %s", tc.arch, ii.arch)
			return
		}

		if ii.kind != tc.kind {
			t.Errorf("kind: expected %d but got %d", tc.kind, ii.kind)
			return
		}
	}
}

func TestImagePatternWithKernel(t *testing.T) {
	envInputs := &env.EnvInputs{
		DeployISO:    "/config/ipa.iso",
		DeployInitrd: "/config/ipa.initramfs",
		DeployKernel: "/config/ipa.kernel",
	}

	tcs := []struct {
		name     string
		filename string
		arch     string
		kind     imageKind
		error    bool
	}{
		{
			name:     "host kernel - exact path match",
			filename: "/config/ipa.kernel",
			arch:     "host",
			kind:     imageKindKernel,
		},
		{
			name:     "aarch64 kernel - underscore separator",
			filename: "/config/ipa_aarch64.kernel",
			arch:     "aarch64",
			kind:     imageKindKernel,
		},
		{
			name:     "x86_64 kernel - period separator",
			filename: "/images/ipa.x86_64.kernel",
			arch:     "x86_64",
			kind:     imageKindKernel,
		},
		{
			name:     "host iso still works",
			filename: "/config/ipa.iso",
			arch:     "host",
			kind:     imageKindISO,
		},
		{
			name:     "host initramfs still works",
			filename: "/config/ipa.initramfs",
			arch:     "host",
			kind:     imageKindInitramfs,
		},
		{
			name:     "invalid kernel - different base name",
			filename: "/images/different-file_x86_64.kernel",
			error:    true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ii, err := loadOSImage(envInputs, tc.filename)

			if err != nil && !tc.error {
				t.Errorf("got error: %v", err)
				return
			}

			if err == nil && tc.error {
				t.Errorf("expected error but got none")
				return
			}

			if tc.error {
				return
			}

			if ii.arch != tc.arch {
				t.Errorf("arch: expected %s but got %s", tc.arch, ii.arch)
			}

			if ii.kind != tc.kind {
				t.Errorf("kind: expected %d but got %d", tc.kind, ii.kind)
			}
		})
	}
}

func TestImagePatternBaseImagesOutsideSharedDir(t *testing.T) {
	// Test that base deploy images can be located anywhere, not just in ImageSharedDir
	envInputs := &env.EnvInputs{
		DeployISO:      "/config/base/ipa.iso",       // Outside shared dir
		DeployInitrd:   "/config/base/ipa.initramfs", // Outside shared dir
		ImageSharedDir: "/shared/images",             // Different directory
	}

	tcs := []struct {
		name     string
		filename string
		arch     string
		kind     imageKind
		error    bool
	}{
		{
			name:     "base iso outside shared dir",
			filename: "/config/base/ipa.iso",
			arch:     "host",
			kind:     imageKindISO,
		},
		{
			name:     "base initramfs outside shared dir",
			filename: "/config/base/ipa.initramfs",
			arch:     "host",
			kind:     imageKindInitramfs,
		},
		{
			name:     "arch-specific iso in shared dir",
			filename: "/shared/images/ipa_aarch64.iso",
			arch:     "aarch64",
			kind:     imageKindISO,
		},
		{
			name:     "arch-specific initramfs in shared dir",
			filename: "/shared/images/ipa.x86_64.initramfs",
			arch:     "x86_64",
			kind:     imageKindInitramfs,
		},
	}

	for _, tc := range tcs {
		t.Logf("testing %s", tc.name)
		ii, err := loadOSImage(envInputs, tc.filename)

		if err != nil && !tc.error {
			t.Errorf("got error: %v", err)
			return
		}

		if err == nil && tc.error {
			t.Errorf("expected error but got none")
			return
		}

		if tc.error {
			continue
		}

		if ii.arch != tc.arch {
			t.Errorf("arch: expected %s but got %s", tc.arch, ii.arch)
			return
		}

		if ii.kind != tc.kind {
			t.Errorf("kind: expected %d but got %d", tc.kind, ii.kind)
			return
		}
	}
}

func TestImagePatternAutoDiscovery(t *testing.T) {
	envInputs := &env.EnvInputs{
		DeployISO:    "/config/base/ipa.iso",
		DeployInitrd: "/opt/images/ipa.initramfs",
	}

	tcs := []struct {
		name     string
		filename string
		arch     string
		kind     imageKind
		error    bool
	}{
		{
			name:     "base iso auto-discovery",
			filename: "/config/base/ipa.iso",
			arch:     "host",
			kind:     imageKindISO,
		},
		{
			name:     "base initramfs auto-discovery",
			filename: "/opt/images/ipa.initramfs",
			arch:     "host",
			kind:     imageKindInitramfs,
		},
		{
			name:     "arch-specific iso in base iso directory",
			filename: "/config/base/ipa_aarch64.iso",
			arch:     "aarch64",
			kind:     imageKindISO,
		},
		{
			name:     "arch-specific initramfs in base initramfs directory",
			filename: "/opt/images/ipa.x86_64.initramfs",
			arch:     "x86_64",
			kind:     imageKindInitramfs,
		},
		{
			name:     "invalid filename in auto-discovered directory",
			filename: "/config/base/different-file_x86_64.iso",
			error:    true,
		},
	}

	for _, tc := range tcs {
		t.Logf("testing %s", tc.name)
		ii, err := loadOSImage(envInputs, tc.filename)

		if err != nil && !tc.error {
			t.Errorf("got error: %v", err)
			return
		}

		if err == nil && tc.error {
			t.Errorf("expected error but got none")
			return
		}

		if tc.error {
			continue
		}

		if ii.arch != tc.arch {
			t.Errorf("arch: expected %s but got %s", tc.arch, ii.arch)
			return
		}

		if ii.kind != tc.kind {
			t.Errorf("kind: expected %d but got %d", tc.kind, ii.kind)
			return
		}
	}
}

func TestArchitectureFallback(t *testing.T) {
	tempDir := t.TempDir()

	envInputs := &env.EnvInputs{
		DeployISO:      filepath.Join(tempDir, "ipa.iso"),
		DeployInitrd:   filepath.Join(tempDir, "ipa.initramfs"),
		ImageSharedDir: tempDir,
	}

	// Create host images only (no architecture-specific images)
	err := os.WriteFile(envInputs.DeployISO, []byte("test iso"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(envInputs.DeployInitrd, []byte("test initramfs"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	logger := zap.New(zap.UseDevMode(true))
	handler, err := NewImageHandler(logger, baseUrl, envInputs)
	if err != nil {
		t.Fatal(err)
	}

	// Get the host architecture that should trigger fallback
	hostArch := env.HostArchitecture()

	// Test that host architecture is supported
	if !handler.HasImagesForArchitecture(hostArch) {
		t.Errorf("Expected HasImagesForArchitecture to return true for host architecture %s, but got false", hostArch)
	}

	// Test ISO fallback - should succeed because it falls back to host image
	isoURL, err := handler.ServeImage("test-key", hostArch, []byte{}, false, false)
	if err != nil {
		t.Errorf("Expected ISO fallback to succeed for arch %s, got error: %v", hostArch, err)
	}
	if isoURL == "" {
		t.Errorf("Expected ISO URL for arch %s, got empty string", hostArch)
	}

	// Test initramfs fallback - should succeed because it falls back to host image
	initramfsURL, err := handler.ServeImage("test-key-initramfs", hostArch, []byte{}, true, false)
	if err != nil {
		t.Errorf("Expected initramfs fallback to succeed for arch %s, got error: %v", hostArch, err)
	}
	if initramfsURL == "" {
		t.Errorf("Expected initramfs URL for arch %s, got empty string", hostArch)
	}

	// Test that non-host architecture is not supported
	nonHostArch := "some_other_arch"
	if nonHostArch == hostArch {
		nonHostArch = "definitely_not_host_arch"
	}

	if handler.HasImagesForArchitecture(nonHostArch) {
		t.Errorf("Expected HasImagesForArchitecture to return false for non-host architecture %s, but got true", nonHostArch)
	}
}

func TestHasImagesForArchitecture(t *testing.T) {
	tempDir := t.TempDir()

	envInputs := &env.EnvInputs{
		DeployISO:      filepath.Join(tempDir, "ipa.iso"),
		DeployInitrd:   filepath.Join(tempDir, "ipa.initramfs"),
		ImageSharedDir: tempDir,
	}

	// Create host images only (no architecture-specific images)
	err := os.WriteFile(envInputs.DeployISO, []byte("test iso"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(envInputs.DeployInitrd, []byte("test initramfs"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Create architecture-specific images for aarch64
	err = os.WriteFile(filepath.Join(tempDir, "ipa_aarch64.iso"), []byte("aarch64 iso"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "ipa_aarch64.initramfs"), []byte("aarch64 initramfs"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	logger := zap.New(zap.UseDevMode(true))
	handler, err := NewImageHandler(logger, baseUrl, envInputs)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		arch      string
		supported bool
		desc      string
	}{
		{"aarch64", true, "aarch64 with both ISO and initramfs files"},
		{"ppc64le", false, "ppc64le with no files available"},
		{env.HostArchitecture(), true, "host architecture with fallback to host files"},
		{"unsupported_arch", false, "unsupported architecture"},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			supported := handler.HasImagesForArchitecture(tc.arch)
			if supported != tc.supported {
				t.Errorf("HasImagesForArchitecture(%s): expected %t, got %t", tc.arch, tc.supported, supported)
			}
		})
	}
}

func TestHasImagesForArchitectureWithKernel(t *testing.T) {
	// Verify that having a kernel file does NOT affect HasImagesForArchitecture
	tempDir := t.TempDir()

	envInputs := &env.EnvInputs{
		DeployISO:      filepath.Join(tempDir, "ipa.iso"),
		DeployInitrd:   filepath.Join(tempDir, "ipa.initramfs"),
		DeployKernel:   filepath.Join(tempDir, "ipa.kernel"),
		ImageSharedDir: tempDir,
	}

	// Create host images
	err := os.WriteFile(envInputs.DeployISO, []byte("test iso"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(envInputs.DeployInitrd, []byte("test initramfs"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(envInputs.DeployKernel, []byte("test kernel"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Create aarch64 kernel only (no ISO/initramfs for aarch64)
	err = os.WriteFile(filepath.Join(tempDir, "ipa_aarch64.kernel"), []byte("aarch64 kernel"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	logger := zap.New(zap.UseDevMode(true))
	handler, err := NewImageHandler(logger, baseUrl, envInputs)
	if err != nil {
		t.Fatal(err)
	}

	// aarch64 has only kernel, not ISO+initramfs, so should NOT be supported
	if handler.HasImagesForArchitecture("aarch64") {
		t.Error("HasImagesForArchitecture should return false for aarch64 with only kernel file")
	}

	// host arch should still be supported (has ISO + initramfs)
	if !handler.HasImagesForArchitecture(env.HostArchitecture()) {
		t.Error("HasImagesForArchitecture should return true for host architecture")
	}
}

func TestServeKernel(t *testing.T) {
	tempDir := t.TempDir()

	// Create kernel files
	kernelPath := filepath.Join(tempDir, "ipa.kernel")
	err := os.WriteFile(kernelPath, []byte("test kernel content"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	aarch64KernelPath := filepath.Join(tempDir, "ipa_aarch64.kernel")
	err = os.WriteFile(aarch64KernelPath, []byte("aarch64 kernel content"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	logger := zap.New(zap.UseDevMode(true))

	ifs := &imageFileSystem{
		log:            logger,
		isoFiles:       map[string]*baseIso{},
		initramfsFiles: map[string]*baseInitramfs{},
		kernelFiles: map[string]*baseKernel{
			"host":    newBaseKernel(kernelPath),
			"aarch64": newBaseKernel(aarch64KernelPath),
		},
		baseURL: baseUrl,
		keys:    map[string]string{},
		images:  map[string]*imageFile{},
		mu:      &sync.Mutex{},
	}

	// Test serving kernel for aarch64
	kernelURL, err := ifs.ServeKernel("aarch64")
	if err != nil {
		t.Fatalf("unexpected error serving aarch64 kernel: %v", err)
	}
	expected := "http://base.test:1234/kernel-aarch64"
	if kernelURL != expected {
		t.Errorf("unexpected kernel URL: got %s, want %s", kernelURL, expected)
	}

	// Test serving kernel for host architecture (falls back to "host" key)
	hostKernelURL, err := ifs.ServeKernel(env.HostArchitecture())
	if err != nil {
		t.Fatalf("unexpected error serving host kernel: %v", err)
	}
	if hostKernelURL == "" {
		t.Error("expected non-empty kernel URL for host architecture")
	}

	// Test serving kernel for unsupported architecture returns empty string
	noKernelURL, err := ifs.ServeKernel("ppc64le")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noKernelURL != "" {
		t.Errorf("expected empty kernel URL for unsupported arch, got %s", noKernelURL)
	}

	// Test idempotency - serving same arch again returns same URL
	kernelURLAgain, err := ifs.ServeKernel("aarch64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kernelURLAgain != kernelURL {
		t.Errorf("inconsistent kernel URLs: %s vs %s", kernelURL, kernelURLAgain)
	}
}

func TestServeKernelNoKernelConfigured(t *testing.T) {
	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	logger := zap.New(zap.UseDevMode(true))

	ifs := &imageFileSystem{
		log:            logger,
		isoFiles:       map[string]*baseIso{},
		initramfsFiles: map[string]*baseInitramfs{},
		kernelFiles:    map[string]*baseKernel{},
		baseURL:        baseUrl,
		keys:           map[string]string{},
		images:         map[string]*imageFile{},
		mu:             &sync.Mutex{},
	}

	// With no kernel files, ServeKernel should return empty string
	kernelURL, err := ifs.ServeKernel("x86_64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kernelURL != "" {
		t.Errorf("expected empty kernel URL when no kernels configured, got %s", kernelURL)
	}
}
