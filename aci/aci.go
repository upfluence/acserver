package aci

import (
	"strings"
	"time"
)

type Aci struct {
	Name    string
	Details []AciDetails
}

type AciDetails struct {
	Version string
	OS      string
	Arch    string
	Signed  bool
	LastMod string
}

type RawFile struct {
	Name string
	Date time.Time
}

func BuildAciList(files []RawFile) []Aci {
	var (
		r             = []Aci{}
		aciDetails    = map[string][]AciDetails{}
		gatheredFiles = map[string]struct {
			aci *RawFile
			asc *RawFile
		}{}
	)

	for _, f := range files {
		if strings.HasSuffix(f.Name, ".asc") {
			v := gatheredFiles[strings.TrimSuffix(f.Name, ".asc")]
			v.asc = &f

			gatheredFiles[strings.TrimSuffix(f.Name, ".asc")] = v
		} else {
			v := gatheredFiles[f.Name]
			v.aci = &f

			gatheredFiles[f.Name] = v
		}
	}

	for name, files := range gatheredFiles {
		if files.aci == nil {
			continue
		}

		tokens := strings.Split(name, "-")
		if len(tokens) != 4 {
			continue
		}

		tokens1 := strings.Split(tokens[3], ".")
		if len(tokens1) != 2 {
			continue
		}

		if tokens1[1] != "aci" {
			continue
		}

		aciDetails[tokens[0]] = append(
			aciDetails[tokens[0]],
			AciDetails{
				Version: tokens[1],
				OS:      tokens[2],
				Arch:    tokens1[0],
				Signed:  files.asc != nil,
				LastMod: files.aci.Date.Format(time.RubyDate),
			},
		)
	}

	for name, details := range aciDetails {
		r = append(r, Aci{name, details})
	}

	return r
}
