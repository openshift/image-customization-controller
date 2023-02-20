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
	"io/fs"
	"net/http"
	"time"
)

func notImplementedFn(name string) error { return fmt.Errorf("%s not implemented", name) }

// file interface implementation

var _ fs.File = &imageFile{}

func (f *imageFileSystem) Stat() (fs.FileInfo, error)        { return fs.FileInfo(f), nil }
func (f *imageFileSystem) Read(p []byte) (n int, err error)  { return 0, notImplementedFn("Read") }
func (f *imageFileSystem) Write(p []byte) (n int, err error) { return 0, notImplementedFn("Write") }
func (f *imageFileSystem) Close() error                      { return nil }

func (f *imageFileSystem) Seek(offset int64, whence int) (int64, error) {
	return 0, notImplementedFn("Seek")
}

func (f *imageFileSystem) Readdir(n int) ([]fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := []fs.FileInfo{}
	for _, im := range f.images {
		result = append(result, im)
	}
	return result, nil
}

func (f *imageFileSystem) Open(name string) (http.File, error) {
	f.log.Info("Open", "path", name)
	if name == "/" {
		return f, nil
	}

	im := f.imageFileByName(name)
	if im == nil {
		return nil, fs.ErrNotExist
	}
	if err := im.Init(f.getBaseImage(im.initramfs)); err != nil {
		f.log.Error(err, "failed to create image stream")
		return nil, err
	}
	return im, nil
}

// fileInfo interface implementation

var _ fs.FileInfo = &imageFileSystem{}

func (i *imageFileSystem) Name() string       { return "/" }
func (i *imageFileSystem) Size() int64        { return 0 }
func (i *imageFileSystem) Mode() fs.FileMode  { return 0755 }
func (i *imageFileSystem) ModTime() time.Time { return time.Now() }
func (i *imageFileSystem) IsDir() bool        { return true }
func (i *imageFileSystem) Sys() interface{}   { return nil }
