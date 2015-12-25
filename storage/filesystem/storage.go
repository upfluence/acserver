package filesystem

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"

	"github.com/appc/acserver/aci"
	"github.com/appc/acserver/storage"
	"github.com/appc/acserver/upload"
)

type Storage struct {
	directory string
	gpgPubKey *string
}

func NewStorage(directory string, gpgPubKey *string) (*Storage, error) {
	os.RemoveAll(path.Join(directory, "tmp"))

	if err := os.MkdirAll(path.Join(directory, "tmp"), 0755); err != nil {
		return nil, err
	}

	return &Storage{directory, gpgPubKey}, nil
}

func (s *Storage) GetGPGPubKey() ([]byte, error) {
	if s.gpgPubKey == nil {
		return []byte{}, storage.ErrGPGPubKeyNotProvided
	}

	return ioutil.ReadFile(*s.gpgPubKey)
}

func (s *Storage) ListACIs() ([]aci.Aci, error) {
	files, err := ioutil.ReadDir(s.directory)
	if err != nil {
		return nil, err
	}

	res := []aci.RawFile{}
	for _, file := range files {
		res = append(
			res,
			aci.RawFile{file.Name(), file.ModTime()},
		)
	}

	return aci.BuildAciList(res), nil
}

func (s *Storage) upload(path string, reader io.Reader) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)

	if err != nil {
		return err
	}

	defer f.Close()

	_, err = io.Copy(f, reader)

	return err
}

func (s *Storage) UploadACI(up upload.Upload, reader io.Reader) error {
	return s.upload(
		path.Join(s.directory, "tmp", strconv.Itoa(int(up.ID))),
		reader,
	)
}

func (s *Storage) UploadASC(up upload.Upload, reader io.Reader) error {
	return s.upload(
		path.Join(s.directory, "tmp", strconv.Itoa(int(up.ID))+".asc"),
		reader,
	)
}

func (s *Storage) CancelUpload(up upload.Upload) error {
	os.Remove(path.Join(s.directory, "tmp", strconv.Itoa(int(up.ID))+".asc"))
	os.Remove(path.Join(s.directory, "tmp", strconv.Itoa(int(up.ID))))

	return nil
}

func (s *Storage) FinishUpload(up upload.Upload) error {
	if err := os.Rename(
		path.Join(s.directory, "tmp", strconv.Itoa(int(up.ID))),
		path.Join(s.directory, up.Image),
	); err != nil {
		return err
	}

	if err := os.Rename(
		path.Join(s.directory, "tmp", strconv.Itoa(int(up.ID))+".asc"),
		path.Join(s.directory, up.Image+".asc"),
	); err != nil {
		return err
	}

	return nil
}

func (s *Storage) DownloadACI(n string) (io.ReadSeeker, error) {
	return os.Open(path.Join(s.directory, n))
}
