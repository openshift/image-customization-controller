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

package main

import (
	"io/fs"
	"net/http"
	"reflect"
	"testing"
	"testing/fstest"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/openshift/image-customization-controller/pkg/env"
	"github.com/openshift/image-customization-controller/pkg/imagehandler"
)

type fakeImageFileSystem struct {
	imagesServed []string
}

var _ imagehandler.ImageHandler = &fakeImageFileSystem{}
var _ http.FileSystem = &fakeImageFileSystem{}

func (f *fakeImageFileSystem) Stat() (fs.FileInfo, error)                   { return nil, nil }
func (f *fakeImageFileSystem) Read(p []byte) (n int, err error)             { return 0, nil }
func (f *fakeImageFileSystem) Write(p []byte) (n int, err error)            { return 0, nil }
func (f *fakeImageFileSystem) Close() error                                 { return nil }
func (f *fakeImageFileSystem) Seek(offset int64, whence int) (int64, error) { return 0, nil }
func (f *fakeImageFileSystem) Readdir(n int) ([]fs.FileInfo, error)         { return nil, nil }
func (f *fakeImageFileSystem) Open(name string) (http.File, error)          { return nil, nil }
func (f *fakeImageFileSystem) FileSystem() http.FileSystem                  { return f }
func (f *fakeImageFileSystem) ServeImage(name string, arch string, ignitionContent []byte, initrd, static bool) (string, error) {
	f.imagesServed = append(f.imagesServed, name)
	return "", nil
}
func (f *fakeImageFileSystem) RemoveImage(name string)                   {}
func (f *fakeImageFileSystem) HasImagesForArchitecture(arch string) bool { return true }

func TestLoadStaticNMState(t *testing.T) {
	fifs := &fakeImageFileSystem{imagesServed: []string{}}
	env := &env.EnvInputs{
		DeployISO:        "foo.iso",
		IronicBaseURL:    "http://example.com",
		IronicAgentImage: "quay.io/tantsur/ironic-agent",
	}

	fs := fstest.MapFS{
		"run/secrets/pull-secret": {},
		"tmp/nmstate/nm0":         {},
		"tmp/nmstate/nm1":         {},
		"tmp/nmstate/nm2":         {},
	}

	if err := loadStaticNMState(fs, env, "/tmp/nmstate/", fifs); err != nil {
		t.Errorf("loadStaticNMState() error = %v", err)
	}
	if !reflect.DeepEqual(fifs.imagesServed, []string{"nm0.iso", "nm0.initramfs", "nm1.iso", "nm1.initramfs", "nm2.iso", "nm2.initramfs"}) {
		t.Errorf("loadStaticNMState() images = %v", fifs.imagesServed)
	}
}
