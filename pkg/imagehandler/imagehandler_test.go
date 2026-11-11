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
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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

	rr := httptest.NewRecorder()
	imageServer := testImageFileSystem()

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

func testImageFileSystem() *imageFileSystem {
	baseURL, _ := url.Parse("http://localhost:8080")
	return &imageFileSystem{
		log:           zap.New(zap.UseDevMode(true)),
		isoFile:       &baseIso{baseFileData{filename: "dummyfile.iso", size: 12345}},
		initramfsFile: newBaseInitramfs("dummyfile.initramfs"),
		baseURL:       baseURL,
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
}

func TestImageHandlerRejectsDirectoryListing(t *testing.T) {
	imageServer := testImageFileSystem()
	handler := http.FileServer(imageServer.FileSystem())

	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("GET / returned status %v, want %v", rr.Code, http.StatusForbidden)
	}
	if strings.Contains(rr.Body.String(), "host-xyz-45-uuid") {
		t.Errorf("GET / leaked image name in body: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/host-xyz-45-uuid", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET of known image returned status %v, want %v", rr.Code, http.StatusOK)
	}
}

func TestImageFileSystemOpenRootDenied(t *testing.T) {
	imageServer := testImageFileSystem()

	_, err := imageServer.Open("/")
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("Open(/) error = %v, want %v", err, fs.ErrPermission)
	}

	_, err = imageServer.Readdir(-1)
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("Readdir error = %v, want %v", err, fs.ErrPermission)
	}
}

func TestNewImageHandler(t *testing.T) {
	baseUrl, err := url.Parse("http://base.test:1234")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	handler := NewImageHandler(zap.New(zap.UseDevMode(true)),
		"dummyfile.iso",
		"dummyfile.initramfs",
		baseUrl)

	ifs := handler.(*imageFileSystem)
	ifs.isoFile.size = 12345
	ifs.initramfsFile.size = 12345

	url1, err := handler.ServeImage("test-key-1", []byte{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	url2, err := handler.ServeImage("test-key-2", []byte{}, true, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	name2 := url2[22:]
	if ifs.imageFileByName(name2) == nil {
		t.Errorf("can't look up image file \"%s\"", name2)
	}

	url1again, err := handler.ServeImage("test-key-1", []byte{}, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	if url1again != url1 {
		t.Errorf("inconsistent URLs for same key: %s %s", url1, url1again)
	}

	handler.RemoveImage("test-key-1")
	url1yetagain, err := handler.ServeImage("test-key-1", []byte{}, false, false)
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
	handler := NewImageHandler(zap.New(zap.UseDevMode(true)),
		"dummyfile.iso",
		"dummyfile.initramfs",
		baseUrl)

	ifs := handler.(*imageFileSystem)
	ifs.isoFile.size = 12345
	ifs.initramfsFile.size = 12345

	url1, err := handler.ServeImage("test-name-1.iso", []byte{}, false, true)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	url2, err := handler.ServeImage("test-name-2.initramfs", []byte{}, true, true)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	url1again, err := handler.ServeImage("test-name-1.iso", []byte{}, false, true)
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
