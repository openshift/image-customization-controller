package imagehandler

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/pkg/errors"

	"github.com/openshift/assisted-image-service/pkg/isoeditor"
)

type baseFile interface {
	Size() (int64, error)
	CheckSum() (string, error)
	InsertIgnition(*isoeditor.IgnitionContent) (isoeditor.ImageReader, error)
}

type baseFileData struct {
	filename string
	size     int64
	checkSum string
}

func (bf *baseFileData) Size() (int64, error) {
	if bf.size == 0 {
		fi, err := os.Stat(bf.filename)
		if err != nil {
			return 0, err
		}
		bf.size = fi.Size()
	}
	return bf.size, nil
}

func (bf *baseFileData) CheckSum() (string, error) {
	if bf.checkSum == "" {
		fp, err := os.Open(bf.filename)
		if err != nil {
			return "", err
		}
		defer fp.Close()

		hash := sha256.New()
		if _, err := io.Copy(hash, fp); err != nil {
			return "", errors.Wrapf(err, "cannot calculate checksum for %s", bf.filename)
		}

		bf.checkSum = hex.EncodeToString(hash.Sum(nil))
	}
	return bf.checkSum, nil
}

type baseIso struct {
	baseFileData
}

func newBaseIso(filename string) *baseIso {
	return &baseIso{baseFileData{filename: filename}}
}

func (biso *baseIso) InsertIgnition(ignition *isoeditor.IgnitionContent) (isoeditor.ImageReader, error) {
	return isoeditor.NewRHCOSStreamReader(biso.filename, ignition, nil)
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
