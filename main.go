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
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/appc/acserver/Godeps/_workspace/src/github.com/upfluence/goamz/aws"
	"github.com/appc/acserver/aci"
	"github.com/appc/acserver/storage"
	"github.com/appc/acserver/storage/s3"
	"github.com/appc/acserver/upload"
	"github.com/appc/acserver/upload/etcd"

	"github.com/appc/acserver/Godeps/_workspace/src/github.com/codegangsta/negroni"
	"github.com/appc/acserver/Godeps/_workspace/src/github.com/gorilla/mux"
)

type initiateDetails struct {
	ACIPushVersion string `json:"aci_push_version"`
	Multipart      bool   `json:"multipart"`
	ManifestURL    string `json:"upload_manifest_url"`
	SignatureURL   string `json:"upload_signature_url"`
	ACIURL         string `json:"upload_aci_url"`
	CompletedURL   string `json:"completed_url"`
}

type completeMsg struct {
	Success      bool   `json:"success"`
	Reason       string `json:"reason,omitempty"`
	ServerReason string `json:"server_reason,omitempty"`
}

type handler struct {
	Fn func(http.ResponseWriter, *http.Request)
}

func (h handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.Fn(rw, req)
}

var (
	serverName  string
	directory   string
	templatedir string
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
	templatedir = args[2]
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

	r := mux.NewRouter()
	r.HandleFunc("/", renderListOfACIs)
	r.HandleFunc("/pubkeys.gpg", getPubkeys)

	n0 := negroni.New()
	n0.UseHandler(handler{initiateUpload})
	r.Handle("/{image}/startupload", n0)

	n1 := negroni.New()
	n1.UseHandler(handler{uploadManifest})
	r.Handle("/manifest/{num}", n1)

	n2 := negroni.New()
	n2.UseHandler(handler{uploadASC})
	r.Handle("/signature/{num}", n2)

	n3 := negroni.New()
	n3.UseHandler(handler{uploadACI})
	r.Handle("/aci/{num}", n3)

	n4 := negroni.New()
	n4.UseHandler(handler{completeUpload})
	r.Handle("/complete/{num}", n4)

	n := negroni.New(negroni.NewStatic(http.Dir(directory)),
		negroni.NewRecovery(), negroni.NewLogger())
	n.UseHandler(r)
	n.Run(":" + strconv.Itoa(*port))
}

// The root page. Builds a human-readable list of what ACIs are available,
// and also provides the meta tags for the server for meta discovery.
func renderListOfACIs(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	t, err := template.ParseFiles(path.Join(templatedir, "index.html"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}
	acis, err := store.ListACIs()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}
	err = t.Execute(w, struct {
		ServerName string
		ACIs       []aci.Aci
		HTTPS      bool
	}{
		ServerName: serverName,
		ACIs:       acis,
		HTTPS:      *https,
	})

	if err != nil {
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
	}
}

// Sends the gpg public keys specified via the flag to the client
func getPubkeys(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	gpgKey, err := store.GetGPGPubKey()

	if err != nil {
		if err == storage.ErrGPGPubKeyNotProvided {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			fmt.Fprintf(os.Stderr, "error opening gpg public key: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	_, err = w.Write(gpgKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading gpg public key: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func initiateUpload(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	image := mux.Vars(req)["image"]
	if image == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	uploadNum, err := newUpload(image)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(os.Stderr, fmt.Sprintf("%v", err))
		return
	}

	var prefix string
	if *https {
		prefix = "https://" + serverName
	} else {
		prefix = "http://" + serverName
	}

	deets := initiateDetails{
		ACIPushVersion: "0.0.1",
		Multipart:      false,
		ManifestURL:    fmt.Sprintf("%s/manifest/%d", prefix, uploadNum),
		SignatureURL:   fmt.Sprintf("%s/signature/%d", prefix, uploadNum),
		ACIURL:         fmt.Sprintf("%s/aci/%d", prefix, uploadNum),
		CompletedURL:   fmt.Sprintf("%s/complete/%d", prefix, uploadNum),
	}

	blob, err := json.Marshal(deets)
	fmt.Printf("blob: %s\n", blob)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(os.Stderr, fmt.Sprintf("%v", err))
		return
	}

	_, err = w.Write(blob)
	if err != nil {
		fmt.Fprintf(os.Stderr, fmt.Sprintf("%v", err))
		return
	}
}

func uploadASC(w http.ResponseWriter, req *http.Request) {
	if req.Method != "PUT" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	numInt, err := strconv.Atoi(mux.Vars(req)["num"])

	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	num := uint64(numInt)

	up := getUpload(num)

	if up == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err = store.UploadASC(*up, req.Body)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error uploading json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = gotSig(num)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error uploading json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func uploadACI(w http.ResponseWriter, req *http.Request) {
	if req.Method != "PUT" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	numInt, err := strconv.Atoi(mux.Vars(req)["num"])

	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	num := uint64(numInt)

	up := getUpload(num)

	if up == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err = store.UploadACI(*up, req.Body)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error uploading json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = gotACI(num)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error uploading json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func uploadManifest(w http.ResponseWriter, req *http.Request) {
	if req.Method != "PUT" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	num, err := strconv.Atoi(mux.Vars(req)["num"])
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err = gotMan(uint64(num))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(os.Stderr, "%v", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func completeUpload(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	numInt, err := strconv.Atoi(mux.Vars(req)["num"])

	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	num := uint64(numInt)

	up := getUpload(num)
	if up == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(os.Stderr, "body: %s\n", string(body))

	msg := completeMsg{}
	err = json.Unmarshal(body, &msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error unmarshaling json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !msg.Success {
		reportFailure(num, w, "client reported failure", msg.Reason)
		return
	}

	if !up.GotMan {
		reportFailure(num, w, "manifest wasn't uploaded", msg.Reason)
		return
	}

	if !up.GotSig {
		reportFailure(num, w, "signature wasn't uploaded", msg.Reason)
		return
	}

	if !up.GotACI {
		reportFailure(num, w, "ACI wasn't uploaded", msg.Reason)
		return
	}

	//TODO: image verification here

	err = finishUpload(num)
	if err != nil {
		reportFailure(num, w, "Internal Server Error", msg.Reason)
		return
	}

	succmsg := completeMsg{
		Success: true,
	}

	blob, err := json.Marshal(succmsg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = w.Write(blob)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	return
}

func reportFailure(num uint64, w http.ResponseWriter, msg, clientmsg string) error {
	err := abortUpload(num)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	failmsg := completeMsg{
		Success:      false,
		Reason:       clientmsg,
		ServerReason: msg,
	}

	blob, err := json.Marshal(failmsg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	_, err = w.Write(blob)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}
	return nil
}

func abortUpload(num uint64) error {
	u, err := backend.Get(num)

	if err != nil {
		return nil
	}

	err = backend.Delete(u.ID)

	if err != nil {
		return nil
	}

	return store.CancelUpload(*u)
}

func finishUpload(num uint64) error {
	u, err := backend.Get(num)

	if err != nil {
		return nil
	}

	err = backend.Delete(u.ID)

	if err != nil {
		return nil
	}

	return store.FinishUpload(*u)
}

func newUpload(image string) (uint64, error) {
	u, err := backend.Create(image)

	if err != nil {
		fmt.Println(err.Error())
		return 0, err
	}

	return u.ID, nil
}

func getUpload(num uint64) *upload.Upload {
	if u, err := backend.Get(num); err == nil {
		return u
	} else {
		return nil
	}
}

func gotSig(num uint64) error {
	u, err := backend.Get(num)

	if err != nil {
		return err
	}

	u.GotSig = true
	return backend.Update(u)
	return nil
}

func gotACI(num uint64) error {
	u, err := backend.Get(num)

	if err != nil {
		return err
	}

	u.GotACI = true
	return backend.Update(u)
}

func gotMan(num uint64) error {
	u, err := backend.Get(num)

	if err != nil {
		return err
	}

	u.GotMan = true
	return backend.Update(u)
}
