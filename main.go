// Copyright 2015 The appc Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/appc/acserver/api"
	"github.com/appc/acserver/storage"
	"github.com/appc/acserver/storage/s3"
	"github.com/appc/acserver/upload"
	"github.com/appc/acserver/upload/etcd"

	"github.com/appc/acserver/Godeps/_workspace/src/github.com/gorilla/handlers"
	"github.com/appc/acserver/Godeps/_workspace/src/github.com/upfluence/goamz/aws"
)

var (
	serverName  string
	directory   string
	templateDir string
	store       storage.Storage
	backend     upload.Backend

	gpgpubkey = flag.String("pubkeys", "",
		"Path to gpg public keys images will be signed with")
	https = flag.Bool("https", false,
		"Whether or not to provide https URLs for meta discovery")
	port = flag.Int("port", 3000, "The port to run the server on")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr,
		"acserver SERVER_NAME ACI_DIRECTORY TEMPLATE_DIRECTORY\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	if len(args) != 3 {
		usage()
		return
	}

	if gpgpubkey == nil {
		fmt.Fprintf(os.Stderr, "internal error: gpgpubkey is nil")
		return
	}

	if https == nil {
		fmt.Fprintf(os.Stderr, "internal error: https is nil")
		return
	}

	if port == nil {
		fmt.Fprintf(os.Stderr, "internal error: port is nil")
		return
	}

	serverName = args[0]
	directory = args[1]
	templateDir = args[2]
	auth, err := aws.EnvAuth()

	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		return
	}

	store, err = s3.NewStorage(auth, aws.USEast, "aci-repository")

	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		return
	}

	backend, err = etcd.NewBackend([]string{"http://127.0.0.1:2379"}, "/acis")

	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		return
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		return
	}

	mux := api.NewServerMux(store, backend, templateDir, serverName, *https)
	http.ListenAndServe(
		fmt.Sprintf(":%d", *port),
		handlers.LoggingHandler(os.Stdout, mux),
	)
}
