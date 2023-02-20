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
	"crypto/sha256"
	"fmt"
	"io"
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
	req, err := http.NewRequest("GET", "/base42/host-xyz-45-uuid", nil)
	if err != nil {
		t.Fatal(err)
	}

	baseURL, _ := url.Parse("http://localhost:8080")

	rr := httptest.NewRecorder()
	imageServer := &imageFileSystem{
		log:     zap.New(zap.UseDevMode(true)),
		isoFile: &baseIso{baseFileData{filename: "dummyfile.iso", size: 12345, checkSum: "base42"}},
		baseURL: baseURL,
		keys: map[string]string{
			"/base42/host-xyz-45-uuid": "host-xyz-45.iso",
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
	handler := NewImageHandler(zap.New(zap.UseDevMode(true)),
		"dummyfile.iso",
		"dummyfile.initramfs",
		baseUrl)

	ifs := handler.(*imageFileSystem)
	ifs.isoFile.size = 12345
	ifs.isoFile.checkSum = "ISO"
	ifs.initramfsFile.size = 12345
	ifs.initramfsFile.checkSum = "INITRD"

	testIgn1 := []byte{0x00, 0x01}
	testIgn2 := []byte{0x01, 0x01}
	testIgnChanged := []byte{0x01, 0x02}

	url1, err := handler.ServeImage("test-key-1", testIgn1, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	url2, err := handler.ServeImage("test-key-2", testIgn2, true, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	parsed2, err := url.Parse(url2)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	name2 := parsed2.Path
	if ifs.imageFileByName(name2) == nil {
		t.Errorf("can't look up image file \"%s\"", name2)
	}
	if parsed2.Host != "base.test:1234" {
		t.Errorf("unexpected host %s", parsed2.Host)
	}
	expectedPath2 := fmt.Sprintf("/INITRD/%x", sha256.Sum256(testIgn2))
	if parsed2.Path != expectedPath2 {
		t.Errorf("path mismatch: expected %s, got %s", expectedPath2, parsed2.Path)
	}

	url1again, err := handler.ServeImage("test-key-1", testIgn1, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	if url1again != url1 {
		t.Errorf("inconsistent URLs for same key: %s %s", url1, url1again)
	}

	url1again, err = handler.ServeImage("test-key-1", testIgnChanged, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	if url1again == url1 {
		t.Errorf("URL was not changed with new ignition: %s", url1again)
	}

	handler.RemoveImage("test-key-1")
	url1yetagain, err := handler.ServeImage("test-key-1", testIgnChanged, false, false)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if url1yetagain != url1again {
		t.Errorf("URL was changed after removal: %s %s", url1again, url1yetagain)
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
