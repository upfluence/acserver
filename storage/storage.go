package storage

import (
	"errors"
	"io"

	"github.com/appc/acserver/aci"
	"github.com/appc/acserver/upload"
)

var (
	ErrGPGPubKeyNotProvided = errors.New("GPG Public Key not provided")
)

type Storage interface {
	GetGPGPubKey() ([]byte, error)
	ListACIs() ([]aci.Aci, error)
	UploadACI(upload.Upload, io.Reader) error
	UploadASC(upload.Upload, io.Reader) error
	FinishUpload(upload.Upload) error
	CancelUpload(upload.Upload) error
}
