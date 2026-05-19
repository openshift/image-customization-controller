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

// streamArchKey identifies a base image by its OS stream and CPU architecture.
type streamArchKey struct {
	stream string // e.g. "rhel-9", "rhel-10"; empty = default/legacy
	arch   string // e.g. "x86_64", "aarch64", "host"
}

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

	// Create pattern: basename[_.]ARCH.extension
	patternStr := fmt.Sprintf(`%s[_.](\w+)%s`, regexp.QuoteMeta(baseName), regexp.QuoteMeta(ext))
	pattern := regexp.MustCompile(patternStr)

	matches := pattern.FindStringSubmatch(filepath.Base(targetFilename))
	if len(matches) == 2 {
		arch := matches[1]
		return &arch
	}

	return nil
}

// matchStreamFilename attempts to extract a stream name (and optionally an
// architecture) from a target filename, given a base filename pattern.
//
// It matches filenames of the form:
//
//	baseName-STREAM.ext           → stream=STREAM, arch="host"
//	baseName-STREAM[_.]ARCH.ext   → stream=STREAM, arch=ARCH
//
// Returns nil if the filename does not match the stream pattern.
func matchStreamFilename(baseFilename, targetFilename string) *osImage {
	if baseFilename == "" {
		return nil
	}

	base := filepath.Base(baseFilename)
	ext := filepath.Ext(base)
	baseName := strings.TrimSuffix(base, ext)

	target := filepath.Base(targetFilename)

	// Pattern: baseName-STREAM([_.]ARCH)?ext
	// STREAM is captured as a non-greedy match of any characters after a dash.
	// ARCH is captured as a word after an underscore or period separator.
	patternStr := fmt.Sprintf(`^%s-(.+?)(?:[_.](\w+))?%s$`, regexp.QuoteMeta(baseName), regexp.QuoteMeta(ext))
	pattern := regexp.MustCompile(patternStr)

	matches := pattern.FindStringSubmatch(target)
	if matches == nil {
		return nil
	}

	stream := matches[1]
	arch := matches[2]
	if arch == "" {
		arch = hostArchitectureKey
	}

	return &osImage{
		filename: targetFilename,
		stream:   stream,
		arch:     arch,
	}
}

type imageKind int

const (
	imageKindISO imageKind = iota
	imageKindInitramfs
	imageKindKernel
)

type osImage struct {
	filename string
	arch     string
	stream   string
	kind     imageKind
}

func loadOSImage(envInputs *env.EnvInputs, filename string) (osImage, error) {
	type matcher struct {
		base string
		kind imageKind
	}

	matchers := []matcher{
		{envInputs.DeployISO, imageKindISO},
		{envInputs.DeployInitrd, imageKindInitramfs},
	}
	if envInputs.DeployKernel != "" {
		matchers = append(matchers, matcher{envInputs.DeployKernel, imageKindKernel})
	}

	// Try stream+arch matching first (e.g. ipa-rhel-9_aarch64.iso)
	for _, m := range matchers {
		if img := matchStreamFilename(m.base, filename); img != nil {
			img.kind = m.kind
			return *img, nil
		}
	}

	// Fall back to arch-only matching (e.g. ipa_aarch64.iso, ipa.iso)
	for _, m := range matchers {
		if arch := matchArchFilename(m.base, filename); arch != nil {
			return osImage{
				filename: filename,
				arch:     *arch,
				kind:     m.kind,
			}, nil
		}
	}

	return osImage{}, fmt.Errorf("failed to load os image name: %s", filename)
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
	isoFiles       map[streamArchKey]*baseIso
	initramfsFiles map[streamArchKey]*baseInitramfs
	kernelFiles    map[streamArchKey]*baseKernel
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
	ServeImage(key string, arch string, stream string, ignitionContent []byte, initramfs, static bool) (string, error)
	ServeKernel(arch string, stream string) (string, error)
	RemoveImage(key string)
	HasImagesForArchitecture(arch string) bool
}

func findOSImageCandidates(logger logr.Logger, envInputs *env.EnvInputs, filePaths []string) []string {
	var searchDirs []string

	deployPaths := []string{envInputs.DeployISO, envInputs.DeployInitrd}
	if envInputs.DeployKernel != "" {
		deployPaths = append(deployPaths, envInputs.DeployKernel)
	}

	if envInputs.ImageSharedDir != "" {
		searchDirs = append(searchDirs, envInputs.ImageSharedDir)
		for _, p := range deployPaths {
			if filepath.Dir(p) != envInputs.ImageSharedDir {
				filePaths = append(filePaths, p)
			}
		}
	} else {
		dirSet := make(map[string]bool)
		for _, p := range deployPaths {
			dirSet[filepath.Dir(p)] = true
		}
		for dir := range dirSet {
			searchDirs = append(searchDirs, dir)
		}
	}

	for _, searchDir := range searchDirs {
		imageFiles, err := os.ReadDir(searchDir)
		if err != nil {
			logger.Info("failed to read directory, continuing", "dir", searchDir, "error", err)
			continue
		}
		logger.Info("reading image files", "dir", searchDir, "len", len(imageFiles))
		for _, imageFile := range imageFiles {
			fullPath := path.Join(searchDir, imageFile.Name())
			filePaths = append(filePaths, fullPath)
		}
	}

	return filePaths
}

func NewImageHandler(logger logr.Logger, baseURL *url.URL, envInputs *env.EnvInputs) (ImageHandler, error) {
	filePaths := findOSImageCandidates(logger, envInputs, nil)

	isoFiles := map[streamArchKey]*baseIso{}
	initramfsFiles := map[streamArchKey]*baseInitramfs{}
	kernelFiles := map[streamArchKey]*baseKernel{}

	logger.Info("processing image files", "total", len(filePaths))
	for _, filePath := range filePaths {
		logger.Info("load image", "file", filePath)

		osImage, err := loadOSImage(envInputs, filePath)
		if err != nil {
			logger.Info("failed to load os image, continuing", "file", filePath)
			continue
		}

		key := streamArchKey{stream: osImage.stream, arch: osImage.arch}
		logger.Info("image loaded", "filename", osImage.filename, "arch", osImage.arch, "stream", osImage.stream, "kind", osImage.kind)

		switch osImage.kind {
		case imageKindISO:
			isoFiles[key] = newBaseIso(filePath)
		case imageKindInitramfs:
			initramfsFiles[key] = newBaseInitramfs(filePath)
		case imageKindKernel:
			kernelFiles[key] = newBaseKernel(filePath)
		}
	}

	return &imageFileSystem{
		log:            logger,
		isoFiles:       isoFiles,
		initramfsFiles: initramfsFiles,
		kernelFiles:    kernelFiles,
		baseURL:        baseURL,
		keys:           map[string]string{},
		images:         map[string]*imageFile{},
		mu:             &sync.Mutex{},
	}, nil
}

func (f *imageFileSystem) FileSystem() http.FileSystem {
	return f
}

func (f *imageFileSystem) getBaseImage(arch string, stream string, initramfs bool) baseFile {
	if arch == "" {
		arch = hostArchitectureKey
	}

	f.log.Info("getBaseImage", "arch", arch, "stream", stream, "initramfs", initramfs)

	getFile := func(key streamArchKey) baseFile {
		if initramfs {
			if file, exists := f.initramfsFiles[key]; exists {
				return file
			}
		} else {
			if file, exists := f.isoFiles[key]; exists {
				return file
			}
		}
		return nil
	}

	// Try exact (stream, arch) match
	if file := getFile(streamArchKey{stream: stream, arch: arch}); file != nil {
		return file
	}

	// Fall back to default stream (empty) for the same arch
	if stream != "" {
		if file := getFile(streamArchKey{stream: "", arch: arch}); file != nil {
			return file
		}
	}

	// Fall back to host architecture key
	if arch == env.HostArchitecture() {
		if file := getFile(streamArchKey{stream: stream, arch: hostArchitectureKey}); file != nil {
			return file
		}
		if stream != "" {
			if file := getFile(streamArchKey{stream: "", arch: hostArchitectureKey}); file != nil {
				return file
			}
		}
	}

	return nil
}

func (f *imageFileSystem) getKernel(arch string, stream string) baseFile {
	if arch == "" {
		arch = hostArchitectureKey
	}

	f.log.Info("getKernel", "arch", arch, "stream", stream)

	getFile := func(key streamArchKey) baseFile {
		if file, exists := f.kernelFiles[key]; exists {
			return file
		}
		return nil
	}

	if file := getFile(streamArchKey{stream: stream, arch: arch}); file != nil {
		return file
	}

	if stream != "" {
		if file := getFile(streamArchKey{stream: "", arch: arch}); file != nil {
			return file
		}
	}

	if arch == env.HostArchitecture() {
		if file := getFile(streamArchKey{stream: stream, arch: hostArchitectureKey}); file != nil {
			return file
		}
		if stream != "" {
			if file := getFile(streamArchKey{stream: "", arch: hostArchitectureKey}); file != nil {
				return file
			}
		}
	}

	return nil
}

func (f *imageFileSystem) ServeKernel(arch string, stream string) (string, error) {
	kernel := f.getKernel(arch, stream)
	if kernel == nil {
		return "", nil
	}

	size, err := kernel.Size()
	if err != nil {
		return "", err
	}

	keyParts := []string{"kernel"}
	if stream != "" {
		keyParts = append(keyParts, stream)
	}
	keyParts = append(keyParts, arch)
	key := strings.Join(keyParts, "-")

	f.mu.Lock()
	defer f.mu.Unlock()

	if img, exists := f.images[key]; exists {
		p, err := url.Parse(fmt.Sprintf("/%s", img.name))
		if err != nil {
			return "", err
		}
		return f.baseURL.ResolveReference(p).String(), nil
	}

	name := key
	p, err := url.Parse(fmt.Sprintf("/%s", name))
	if err != nil {
		return "", err
	}

	f.keys[name] = key
	f.images[key] = &imageFile{
		name:   name,
		arch:   arch,
		stream: stream,
		size:   size,
		kernel: true,
	}

	return f.baseURL.ResolveReference(p).String(), nil
}

func (f *imageFileSystem) HasImagesForArchitecture(arch string) bool {
	streams := f.availableStreams()
	for _, stream := range streams {
		if f.getBaseImage(arch, stream, false) != nil && f.getBaseImage(arch, stream, true) != nil {
			return true
		}
	}
	return false
}

func (f *imageFileSystem) availableStreams() []string {
	seen := map[string]struct{}{}
	for key := range f.isoFiles {
		seen[key.stream] = struct{}{}
	}
	for key := range f.initramfsFiles {
		seen[key.stream] = struct{}{}
	}
	streams := make([]string, 0, len(seen))
	for s := range seen {
		streams = append(streams, s)
	}
	return streams
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

func (f *imageFileSystem) ServeImage(key string, arch string, stream string, ignitionContent []byte, initramfs, static bool) (string, error) {
	f.log.Info("ServeImage", "arch", arch, "stream", stream)
	baseImage := f.getBaseImage(arch, stream, initramfs)
	if baseImage == nil {
		return "", InvalidBaseImageError{cause: fmt.Errorf("no base image found for arch=%s stream=%s initramfs=%v", arch, stream, initramfs)}
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
			stream:          stream,
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
