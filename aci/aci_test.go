package aci

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildAciList(t *testing.T) {
	if l := len(BuildAciList([]RawFile{})); l != 0 {
		t.Errorf("Wrong len by default: %d", l)
	}

	date := "Mon Jan 02 15:04:05 -0700 2006"
	d, _ := time.Parse(time.RubyDate, date)

	data := []RawFile{
		RawFile{"foo.com/bar-latest-linux-amd64.aci", d},
		RawFile{"foo.com/bar-latest-linux-amd64.aci.asc", d},
		RawFile{"foo.com/bar-0.0.4-linux-amd64.aci", d},
		RawFile{"foo.com/buz-latest-linux-amd64.aci", d},
		RawFile{"foo.com/fiz-0.0.1-linux-amd64.aci", d},
		RawFile{"foo.com/fuz-wrong", d},
	}

	e := []Aci{
		Aci{
			"foo.com/bar",
			[]AciDetails{
				AciDetails{"latest", "linux", "amd64", true, date},
				AciDetails{"0.0.4", "linux", "amd64", false, date},
			},
		},
		Aci{
			"foo.com/fiz",
			[]AciDetails{
				AciDetails{"0.0.1", "linux", "amd64", false, date},
			},
		},
		Aci{
			"foo.com/buz",
			[]AciDetails{
				AciDetails{"latest", "linux", "amd64", false, date},
			},
		},
	}

	test := BuildAciList(data)
	if !containsAll(test, e...) {
		t.Errorf("Wrong parsing: %+v", test)
	}
}

func containsAll(vs []Aci, elts ...Aci) bool {
	ret := []bool{}

	for _, elt := range elts {
		vS := false
		for _, v := range vs {
			if reflect.DeepEqual(v, elt) {
				vS = true
				break
			}
		}
		ret = append(ret, vS)
	}

	res := true
	for _, r := range ret {
		if !r {
			res = false
		}
	}

	return res
}
