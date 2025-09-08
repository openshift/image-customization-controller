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
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/openshift/image-customization-controller/pkg/env"
)

const (
	hostArchitectureKey = "host"
)

// matchArchFilename attempts to match a target filename against a base filename.
// Returns "host" for exact matches, the architecture for pattern matches, or nil if no match.
func matchArchFilename(baseFilename, targetFilename string) *string {
	if baseFilename == "" {
		return nil
	}

	if targetFilename == baseFilename {
		arch := hostArchitectureKey
		return &arch
	}

	base := filepath.Base(baseFilename)
	ext := filepath.Ext(base)
	baseName := strings.TrimSuffix(base, ext)

	// Create pattern: basename_ARCH.extension
	patternStr := fmt.Sprintf(`%s_(\w+)%s`, regexp.QuoteMeta(baseName), regexp.QuoteMeta(ext))
	pattern := regexp.MustCompile(patternStr)

	matches := pattern.FindStringSubmatch(filepath.Base(targetFilename))
	if len(matches) == 2 {
		arch := matches[1]
		return &arch
	}

	return nil
}

type ironicImage struct {
	filename string
	arch     string
	iso      bool
}

func parseDeployImage(envInputs *env.EnvInputs, filename string) (ironicImage, error) {
	if arch := matchArchFilename(envInputs.DeployISO, filename); arch != nil {
		return ironicImage{
			filename: filename,
			arch:     *arch,
			iso:      true,
		}, nil
	}

	if arch := matchArchFilename(envInputs.DeployInitrd, filename); arch != nil {
		return ironicImage{
			filename: filename,
			arch:     *arch,
			iso:      false,
		}, nil
	}

	return ironicImage{}, fmt.Errorf("failed to parse ironic image name: %s", filename)
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

func NewImageHandler(logger logr.Logger, baseURL *url.URL, envInputs *env.EnvInputs) (ImageHandler, error) {
	imageFiles, err := os.ReadDir(envInputs.ImageSharedDir)

	if err != nil {
		return &imageFileSystem{}, err
	}

	isoFiles := map[string]*baseIso{}
	initramfsFiles := map[string]*baseInitramfs{}

	logger.Info("reading image files", "dir", envInputs.ImageSharedDir, "len", len(imageFiles))
	for _, imageFile := range imageFiles {
		filename := imageFile.Name()

		logger.Info("load image", "imageFile", imageFile.Name())

		ironicImage, err := parseDeployImage(envInputs, filename)
		if err != nil {
			logger.Info("failed to parse ironic image, continuing")
			continue
		}

		logger.Info("image loaded", "filename", ironicImage.filename, "arch", ironicImage.arch, "iso", ironicImage.iso)

		if ironicImage.iso {
			isoFiles[ironicImage.arch] = newBaseIso(path.Join(envInputs.ImageSharedDir, filename))
		} else {
			initramfsFiles[ironicImage.arch] = newBaseInitramfs(path.Join(envInputs.ImageSharedDir, filename))
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

func (f *imageFileSystem) getBaseImage(arch string, initramfs bool) baseFile {
	if arch == "" {
		arch = hostArchitectureKey
	}

	f.log.Info("getBaseImage", "arch", arch, "initramfs", initramfs)
	if initramfs {
		file := f.initramfsFiles[arch]
		return file
	} else {
		file := f.isoFiles[arch]
		return file
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
	baseImage := f.getBaseImage(arch, initramfs)

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
