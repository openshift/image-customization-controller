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
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sync"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

const (
	imageSharedDir = "/shared/html/images"
)

var pythonImagePattern = regexp.MustCompile(`ironic-python-agent-(\w+)\.(iso|initramfs)`)

type ironicImage struct {
	filename  string
	arch      string
	iso       bool
	initramfs bool
}

func parseIronicImage(filename string) (ironicImage, error) {
	if path.Base(filename) == "ironic-python-agent.iso" {
		return ironicImage{
			filename: filename,
			arch:     "host",
			iso:      true,
		}, nil
	}

	if path.Base(filename) == "ironic-python-agent.initramfs" {
		return ironicImage{
			filename:  filename,
			arch:      "host",
			initramfs: true,
		}, nil
	}

	matches := pythonImagePattern.FindStringSubmatch(filename)

	if len(matches) != 3 {
		return ironicImage{}, fmt.Errorf("failed to parse ironic image name: %s", filename)
	}

	return ironicImage{
		filename:  filename,
		arch:      matches[1],
		iso:       matches[2] == "iso",
		initramfs: matches[2] == "initramfs",
	}, nil
}

type InvalidBaseImageError struct {
	cause error
}

func (ie InvalidBaseImageError) Error() string {
	return "Base Image not available"
}

func (ie InvalidBaseImageError) Unwrap() error {
	return ie.cause
}

// imageFileSystem is an http.FileSystem that creates a virtual filesystem of
// host images.
type imageFileSystem struct {
	isoFiles       map[string]*baseIso
	initramfsFiles map[string]*baseInitramfs
	baseURL        *url.URL
	keys           map[string]string
	images         map[string]*imageFile
	mu             *sync.Mutex
	log            logr.Logger
}

var _ ImageHandler = &imageFileSystem{}
var _ http.FileSystem = &imageFileSystem{}

type ImageHandler interface {
	FileSystem() http.FileSystem
	ServeImage(key string, arch string, ignitionContent []byte, initramfs, static bool) (string, error)
	RemoveImage(key string)
}

func NewImageHandler(logger logr.Logger, baseURL *url.URL) (ImageHandler, error) {
	imageFiles, err := os.ReadDir(imageSharedDir)

	if err != nil {
		return &imageFileSystem{}, err
	}

	isoFiles := map[string]*baseIso{}
	initramfsFiles := map[string]*baseInitramfs{}

	logger.Info("reading image files", "dir", imageSharedDir, "len", len(imageFiles))
	for _, imageFile := range imageFiles {
		filename := imageFile.Name()

		logger.Info("load image", "imageFile", imageFile.Name())

		ironicImage, err := parseIronicImage(filename)
		if err != nil {
			logger.Info("failed to parse ironic image, continuing")
			continue
		}

		logger.Info("image loaded", "filename", ironicImage.filename, "arch", ironicImage.arch, "iso", ironicImage.iso, "initramfs", ironicImage.initramfs)

		if ironicImage.iso {
			isoFiles[ironicImage.arch] = newBaseIso(path.Join(imageSharedDir, filename))
		}

		if ironicImage.initramfs {
			initramfsFiles[ironicImage.arch] = newBaseInitramfs(path.Join(imageSharedDir, filename))
		}
	}

	return &imageFileSystem{
		log:            logger,
		isoFiles:       isoFiles,
		initramfsFiles: initramfsFiles,
		baseURL:        baseURL,
		keys:           map[string]string{},
		images:         map[string]*imageFile{},
		mu:             &sync.Mutex{},
	}, nil
}

func (f *imageFileSystem) FileSystem() http.FileSystem {
	return f
}

func (f *imageFileSystem) getBaseImage(arch string, initramfs bool) (baseFile, bool) {
	if arch == "" {
		arch = "host"
	}

	f.log.Info("getBaseImage", "arch", arch, "initramfs", initramfs)
	if initramfs {
		file, found := f.initramfsFiles[arch]
		return file, found
	} else {
		file, found := f.isoFiles[arch]
		return file, found
	}
}

func (f *imageFileSystem) getNameForKey(key string) (name string, err error) {
	if img, exists := f.images[key]; exists {
		return img.name, nil
	}
	rand, err := uuid.NewRandom()
	if err == nil {
		name = rand.String()
	}
	return
}

func (f *imageFileSystem) ServeImage(key string, arch string, ignitionContent []byte, initramfs, static bool) (string, error) {
	f.log.Info("ServeImage")
	baseImage, found := f.getBaseImage(arch, initramfs)

	if !found {
		return "", InvalidBaseImageError{cause: fmt.Errorf("not found")}
	}

	size, err := baseImage.Size()
	if err != nil {
		return "", InvalidBaseImageError{cause: err}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	name := key
	if !static {
		name, err = f.getNameForKey(key)
		if err != nil {
			return "", err
		}
	}
	p, err := url.Parse(fmt.Sprintf("/%s", name))
	if err != nil {
		return "", err
	}

	if _, exists := f.images[key]; !exists {
		f.keys[name] = key
		f.images[key] = &imageFile{
			name:            name,
			arch:            arch,
			size:            size,
			ignitionContent: ignitionContent,
			initramfs:       initramfs,
		}
	}

	return f.baseURL.ResolveReference(p).String(), nil
}

func (f *imageFileSystem) imageFileByName(name string) *imageFile {
	f.mu.Lock()
	defer f.mu.Unlock()

	if key, exists := f.keys[name]; exists {
		return f.images[key]
	}
	return nil
}

func (f *imageFileSystem) RemoveImage(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if img, exists := f.images[key]; exists {
		delete(f.keys, img.name)
		delete(f.images, key)
	}
}
