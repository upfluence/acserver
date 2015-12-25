package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strconv"

	"github.com/appc/acserver/aci"
	"github.com/appc/acserver/storage"
	"github.com/appc/acserver/upload"

	"github.com/appc/acserver/Godeps/_workspace/src/github.com/gorilla/mux"
)

type Mux struct {
	http.Handler

	store   storage.Storage
	backend upload.Backend

	templateDir string
	serverName  string
	https       bool
}

type Handler struct {
	path    string
	handler func(http.ResponseWriter, *http.Request)
}

type completeMsg struct {
	Success      bool   `json:"success"`
	Reason       string `json:"reason,omitempty"`
	ServerReason string `json:"server_reason,omitempty"`
}

type initiateDetails struct {
	ACIPushVersion string `json:"aci_push_version"`
	Multipart      bool   `json:"multipart"`
	ManifestURL    string `json:"upload_manifest_url"`
	SignatureURL   string `json:"upload_signature_url"`
	ACIURL         string `json:"upload_aci_url"`
	CompletedURL   string `json:"completed_url"`
}

func NewServerMux(store storage.Storage, backend upload.Backend, templateDir, serverName string, https bool) *Mux {
	sm := mux.NewRouter()
	mux := &Mux{sm, store, backend, templateDir, serverName, https}

	for _, couple := range []Handler{
		Handler{"/", mux.renderACIs},
		Handler{"/pubkeys.gpg", mux.getPubkeys},
		Handler{"/{image}/startupload", mux.initiateUpload},
		Handler{
			"/manifest/{num}",
			mux.uploadData(
				func(u *upload.Upload, req io.Reader) error { return nil },
				func(u *upload.Upload) { u.GotMan = true },
			),
		},
		Handler{
			"/signature/{num}",
			mux.uploadData(
				func(u *upload.Upload, req io.Reader) error {
					return store.UploadASC(*u, req)
				},
				func(u *upload.Upload) { u.GotSig = true },
			),
		},
		Handler{
			"/aci/{num}",
			mux.uploadData(
				func(u *upload.Upload, req io.Reader) error {
					return store.UploadACI(*u, req)
				},
				func(u *upload.Upload) { u.GotACI = true },
			),
		},
		Handler{"/complete/{num}", mux.completeUpload},
	} {
		sm.HandleFunc(couple.path, couple.handler)
	}

	return mux
}

func (m *Mux) renderACIs(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	t, err := template.ParseFiles(path.Join(m.templateDir, "index.html"))

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	acis, err := m.store.ListACIs()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	if err = t.Execute(w, struct {
		ServerName string
		ACIs       []aci.Aci
		HTTPS      bool
	}{
		ServerName: m.serverName,
		ACIs:       acis,
		HTTPS:      m.https,
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
	}
}

func (m *Mux) getPubkeys(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	gpgKey, err := m.store.GetGPGPubKey()

	if err != nil {
		if err == storage.ErrGPGPubKeyNotProvided {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			fmt.Fprintf(w, "error opening gpg public key: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	if _, err = w.Write(gpgKey); err != nil {
		fmt.Fprintf(w, "error reading gpg public key: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (m *Mux) initiateUpload(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	image := mux.Vars(req)["image"]
	if image == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	upload, err := m.backend.Create(image)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	var prefix string
	if m.https {
		prefix = "https://" + m.serverName
	} else {
		prefix = "http://" + m.serverName
	}

	deets := initiateDetails{
		ACIPushVersion: "0.0.1",
		Multipart:      false,
		ManifestURL:    fmt.Sprintf("%s/manifest/%d", prefix, upload.ID),
		SignatureURL:   fmt.Sprintf("%s/signature/%d", prefix, upload.ID),
		ACIURL:         fmt.Sprintf("%s/aci/%d", prefix, upload.ID),
		CompletedURL:   fmt.Sprintf("%s/complete/%d", prefix, upload.ID),
	}

	blob, err := json.Marshal(deets)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	if _, err = w.Write(blob); err != nil {
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}
}

func (m *Mux) uploadData(uploadData func(*upload.Upload, io.Reader) error, updateUpload func(*upload.Upload)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "PUT" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		num, err := strconv.Atoi(mux.Vars(req)["num"])

		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		up, err := m.backend.Get(uint64(num))

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, fmt.Sprintf("%v", err))
			return
		}

		if err := uploadData(up, req.Body); err != nil {
			fmt.Fprintf(w, fmt.Sprintf("%v", err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		updateUpload(up)

		if err := m.backend.Update(up); err != nil {
			fmt.Fprintf(w, fmt.Sprintf("%v", err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (m *Mux) completeUpload(w http.ResponseWriter, req *http.Request) {
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

	up, err := m.backend.Get(uint64(num))

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	msg := completeMsg{}

	if err = json.Unmarshal(body, &msg); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	if !msg.Success {
		m.reportFailure(up, w, "client reported failure", msg.Reason)
		return
	}

	if !up.GotMan {
		m.reportFailure(up, w, "manifest wasn't uploaded", msg.Reason)
		return
	}

	if !up.GotSig {
		m.reportFailure(up, w, "signature wasn't uploaded", msg.Reason)
		return
	}

	if !up.GotACI {
		m.reportFailure(up, w, "ACI wasn't uploaded", msg.Reason)
		return
	}

	//TODO: image verification here

	if err = m.store.FinishUpload(*up); err != nil {
		m.reportFailure(up, w, "Internal Server Error", msg.Reason)
		return
	} else {
		m.backend.Delete(up.ID)
	}

	blob, err := json.Marshal(completeMsg{Success: true})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}

	if _, err = w.Write(blob); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
		return
	}
}

func (m *Mux) reportFailure(up *upload.Upload, w http.ResponseWriter, msg, clientmsg string) {
	err := m.backend.Delete(up.ID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
	}

	m.store.CancelUpload(*up)

	failmsg := completeMsg{
		Success:      false,
		Reason:       clientmsg,
		ServerReason: msg,
	}

	blob, err := json.Marshal(failmsg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
	}

	if _, err = w.Write(blob); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, fmt.Sprintf("%v", err))
	}
}
