package memory

import (
	"sync"

	"github.com/appc/acserver/upload"
)

type Backend struct {
	mu      sync.Mutex
	store   map[uint64]*upload.Upload
	counter uint64
}

func NewBackend() (*Backend, error) {
	return &Backend{sync.Mutex{}, make(map[uint64]*upload.Upload), 0}, nil
}

func (b *Backend) Create(name string) (*upload.Upload, error) {
	if name == "" {
		return nil, upload.ErrEmptyName
	}

	up := upload.NewUpload(name)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.counter++

	up.ID = b.counter
	b.store[up.ID] = up

	return up, nil
}

func (b *Backend) Get(id uint64) (*upload.Upload, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if v, ok := b.store[id]; ok {
		return v, nil
	}

	return nil, upload.ErrNotFound
}

func (b *Backend) Update(up *upload.Upload) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.store[up.ID]; ok {
		b.store[up.ID] = up

		return nil
	}

	return upload.ErrNotFound
}

func (b *Backend) Delete(id uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.store[id]; ok {
		delete(b.store, id)
		return nil
	}

	return upload.ErrNotFound
}
