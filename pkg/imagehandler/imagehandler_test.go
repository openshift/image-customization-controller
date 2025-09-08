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
		iso      bool
		error    bool
	}{
		{
			name:     "host iso - exact path match",
			filename: "/config/ipa.iso",
			arch:     "host",
			iso:      true,
		},
		{
			name:     "host initramfs - exact path match",
			filename: "/config/ipa.initramfs",
			arch:     "host",
		},
		{
			name:     "aarch64 iso - same directory as config",
			filename: "/config/ipa_aarch64.iso",
			arch:     "aarch64",
			iso:      true,
		},
		{
			name:     "aarch64 initramfs - different directory from config",
			filename: "/images/ipa_aarch64.initramfs",
			arch:     "aarch64",
		},
		{
			name:     "x86_64 iso - different directory from config",
			filename: "/images/ipa_x86_64.iso",
			arch:     "x86_64",
			iso:      true,
		},
		{
			name:     "x86_64 initramfs - relative path",
			filename: "ipa_x86_64.initramfs",
			arch:     "x86_64",
		},
		{
			name:     "aarch64 iso - period separator",
			filename: "ipa.aarch64.iso",
			arch:     "aarch64",
			iso:      true,
		},
		{
			name:     "x86_64 initramfs - period separator",
			filename: "/images/ipa.x86_64.initramfs",
			arch:     "x86_64",
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

		if ii.iso != tc.iso {
			t.Errorf("iso: expected %t but got %t", tc.iso, ii.iso)
			return
		}
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
		iso      bool
		error    bool
	}{
		{
			name:     "base iso outside shared dir",
			filename: "/config/base/ipa.iso",
			arch:     "host",
			iso:      true,
		},
		{
			name:     "base initramfs outside shared dir",
			filename: "/config/base/ipa.initramfs",
			arch:     "host",
		},
		{
			name:     "arch-specific iso in shared dir",
			filename: "/shared/images/ipa_aarch64.iso",
			arch:     "aarch64",
			iso:      true,
		},
		{
			name:     "arch-specific initramfs in shared dir",
			filename: "/shared/images/ipa.x86_64.initramfs",
			arch:     "x86_64",
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

		if ii.iso != tc.iso {
			t.Errorf("iso: expected %t but got %t", tc.iso, ii.iso)
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
		iso      bool
		error    bool
	}{
		{
			name:     "base iso auto-discovery",
			filename: "/config/base/ipa.iso",
			arch:     "host",
			iso:      true,
		},
		{
			name:     "base initramfs auto-discovery",
			filename: "/opt/images/ipa.initramfs",
			arch:     "host",
		},
		{
			name:     "arch-specific iso in base iso directory",
			filename: "/config/base/ipa_aarch64.iso",
			arch:     "aarch64",
			iso:      true,
		},
		{
			name:     "arch-specific initramfs in base initramfs directory",
			filename: "/opt/images/ipa.x86_64.initramfs",
			arch:     "x86_64",
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

		if ii.iso != tc.iso {
			t.Errorf("iso: expected %t but got %t", tc.iso, ii.iso)
			return
		}
	}
}
