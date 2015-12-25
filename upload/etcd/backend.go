package etcd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/appc/acserver/Godeps/_workspace/src/github.com/coreos/etcd/client"
	"github.com/appc/acserver/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/appc/acserver/upload"
)

type Backend struct {
	api client.KeysAPI

	namespace string
}

func NewBackend(endpoints []string, namespace string) (*Backend, error) {
	cfg := client.Config{
		Endpoints: endpoints,
		Transport: client.DefaultTransport,
	}

	c, err := client.New(cfg)

	if err != nil {
		return nil, err
	}

	kapi := client.NewKeysAPI(c)

	_, err = kapi.Create(
		context.Background(),
		fmt.Sprintf("%s/counter", namespace),
		"0",
	)

	if err != nil {
		if e, ok := err.(client.Error); !ok || e.Code != client.ErrorCodeNodeExist {
			return nil, err
		}
	}

	return &Backend{kapi, namespace}, nil
}

func (b *Backend) Get(id uint64) (*upload.Upload, error) {
	n, err := b.api.Get(
		context.Background(),
		fmt.Sprintf("%s/%d", b.namespace, id),
		nil,
	)

	if err != nil {
		return nil, err
	}

	up := upload.Upload{}

	if err := json.Unmarshal([]byte(n.Node.Value), &up); err != nil {
		return nil, err
	}

	return &up, nil
}

func (b *Backend) Create(name string) (*upload.Upload, error) {
	up := upload.NewUpload(name)

	n, err := b.api.Get(
		context.Background(),
		fmt.Sprintf("%s/counter", b.namespace),
		nil,
	)

	if err != nil {
		return nil, err
	}

	v, err := strconv.Atoi(n.Node.Value)

	if err != nil {
		return nil, err
	}

	_, err = b.api.Set(
		context.Background(),
		fmt.Sprintf("%s/counter", b.namespace),
		strconv.Itoa(v+1),
		&client.SetOptions{PrevValue: n.Node.Value},
	)

	if err != nil {
		return nil, err
	}

	up.ID = uint64(v + 1)

	blob, err := json.Marshal(up)

	if err != nil {
		return nil, err
	}

	_, err = b.api.Create(
		context.Background(),
		fmt.Sprintf("%s/%d", b.namespace, v+1),
		string(blob),
	)

	if err != nil {
		return nil, err
	}

	return up, nil
}

func (b *Backend) Update(up *upload.Upload) error {
	blob, err := json.Marshal(up)

	if err != nil {
		return err
	}

	//TODO: Should compare and swap
	_, err = b.api.Set(
		context.Background(),
		fmt.Sprintf("%s/%d", b.namespace, up.ID),
		string(blob),
		nil,
	)

	return err
}

func (b *Backend) Delete(id uint64) error {
	//TODO: Should compare and swap
	_, err := b.api.Delete(
		context.Background(),
		fmt.Sprintf("%s/%d", b.namespace, id),
		nil,
	)

	return err
}
