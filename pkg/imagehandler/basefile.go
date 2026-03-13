package imagehandler

import (
	"fmt"
	"io"
	"os"

	"github.com/openshift/assisted-image-service/pkg/isoeditor"
	"github.com/openshift/assisted-image-service/pkg/overlay"
)

type baseFile interface {
	Size() (int64, error)
	InsertIgnition(*isoeditor.IgnitionContent) (isoeditor.ImageReader, error)
}

type baseFileData struct {
	filename string
	size     int64
}

func (bf *baseFileData) Size() (int64, error) {
	if bf.size != 0 {
		return bf.size, nil
	}

	fi, err := os.Stat(bf.filename)
	if err != nil {
		return 0, err
	}
	if fi.Mode().IsRegular() {
		bf.size = fi.Size()
	} else if fi.Mode()&os.ModeDevice != 0 {
		file, err := os.Open(bf.filename)
		if err != nil {
			return 0, err
		}
		defer file.Close()
		size, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		bf.size = size
	} else {
		return 0, fmt.Errorf("base file %s is not a regular file or block device", bf.filename)
	}
	return bf.size, nil
}

type baseIso struct {
	baseFileData
}

func newBaseIso(filename string) *baseIso {
	return &baseIso{baseFileData{filename: filename}}
}

func (biso *baseIso) InsertIgnition(ignition *isoeditor.IgnitionContent) (isoeditor.ImageReader, error) {
	return isoeditor.NewRHCOSStreamReader(biso.filename, ignition, nil, nil)
}

type baseInitramfs struct {
	baseFileData
}

func newBaseInitramfs(filename string) *baseInitramfs {
	return &baseInitramfs{baseFileData{filename: filename}}
}

func (birfs *baseInitramfs) InsertIgnition(ignition *isoeditor.IgnitionContent) (isoeditor.ImageReader, error) {
	return isoeditor.NewInitRamFSStreamReader(birfs.filename, ignition)
}

type baseKernel struct {
	baseFileData
}

func newBaseKernel(filename string) *baseKernel {
	return &baseKernel{baseFileData{filename: filename}}
}

// InsertIgnition for kernel files returns the file as-is with no modification.
// *os.File satisfies overlay.OverlayReader (io.ReadSeekCloser).
func (bk *baseKernel) InsertIgnition(_ *isoeditor.IgnitionContent) (isoeditor.ImageReader, error) {
	return os.Open(bk.filename)
}

// Ensure *os.File satisfies the ImageReader (overlay.OverlayReader) interface.
var _ overlay.OverlayReader = (*os.File)(nil)
