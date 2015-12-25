package upload

import "errors"

var (
	ErrEmptyName = errors.New("Empty name")
	ErrNotFound  = errors.New("Upload not found")
)

type Backend interface {
	Create(string) (*Upload, error)
	Get(uint64) (*Upload, error)
	Update(*Upload) error
	Delete(uint64) error
}
