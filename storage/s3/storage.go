package s3

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/appc/acserver/Godeps/_workspace/src/github.com/upfluence/goamz/aws"
	"github.com/appc/acserver/Godeps/_workspace/src/github.com/upfluence/goamz/s3"
	"github.com/appc/acserver/aci"
	"github.com/appc/acserver/upload"
)

const (
	gpgPubKeyPath = "keys/key.pub"
	aciPath       = "acis/"
)

type Storage struct {
	*s3.Bucket
}

func NewStorage(auth aws.Auth, region aws.Region, bucket string) (*Storage, error) {
	return &Storage{s3.New(auth, region).Bucket(bucket)}, nil
}

func (s *Storage) GetGPGPubKey() ([]byte, error) {
	return s.Get(gpgPubKeyPath)
}

func (s *Storage) ListACIs() ([]aci.Aci, error) {
	res := []aci.RawFile{}
	r, err := s.List(aciPath, "/", "", 0)

	if err != nil {
		return []aci.Aci{}, err
	}

	for _, c := range r.Contents {
		t, _ := time.Parse(time.RFC3339, c.LastModified)
		res = append(
			res,
			aci.RawFile{strings.TrimPrefix(c.Key, aciPath), t},
		)
	}

	return aci.BuildAciList(res), nil
}

func (s *Storage) upload(path string, reader io.Reader) error {
	buf := &bytes.Buffer{}
	buf.ReadFrom(reader)

	return s.PutReader(
		path,
		buf,
		int64(buf.Len()),
		"application/octet-stream",
		s3.Private,
	)
}

func (s *Storage) UploadACI(up upload.Upload, reader io.Reader) error {
	return s.upload(
		fmt.Sprintf("tmp/%d", up.ID),
		reader,
	)
}

func (s *Storage) UploadASC(up upload.Upload, reader io.Reader) error {
	return s.upload(
		fmt.Sprintf("tmp/%d.asc", up.ID),
		reader,
	)
}

func (s *Storage) deleteTemps(up upload.Upload) error {
	return s.MultiDel(
		[]string{
			fmt.Sprintf("tmp/%d", up.ID),
			fmt.Sprintf("tmp/%d.asc", up.ID),
		},
	)
}

func (s *Storage) CancelUpload(up upload.Upload) error {
	return s.deleteTemps(up)
}

func (s *Storage) FinishUpload(up upload.Upload) error {
	if err := s.Copy(
		fmt.Sprintf("tmp/%d", up.ID),
		fmt.Sprintf("%s%s", aciPath, up.Image),
		s3.Private,
	); err != nil {
		return err
	}

	if err := s.Copy(
		fmt.Sprintf("tmp/%d.asc", up.ID),
		fmt.Sprintf("%s%s.asc", aciPath, up.Image),
		s3.Private,
	); err != nil {
		return err
	}

	return s.deleteTemps(up)
}
