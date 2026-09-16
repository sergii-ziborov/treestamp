package selection

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestWritePolicySeparatesLists(t *testing.T) {
	a := Filters{IncludeNames: []string{"x", "exc-namey"}, ExcludeNames: []string{"z"}}
	b := Filters{IncludeNames: []string{"x"}, ExcludeNames: []string{"y", "exc-namez"}}
	if writePolicyBytes(a) == writePolicyBytes(b) {
		t.Fatal("distinct filters encoded identically")
	}
	if a.rejectFile("exc-namey", "") || !b.rejectFile("exc-namey", "") {
		t.Fatal("selection must still differ")
	}
}

func writePolicyBytes(f Filters) string {
	h := sha256.New()
	f.WritePolicy(h)
	return string(h.Sum(nil))
}

func TestLocationIncludeKeepsPrefixDirs(t *testing.T) {
	f := Filters{LocationInclude: []string{"services/payments/**"}}
	if f.rejectDir("services", "services", "services") {
		t.Fatal("must enter prefix directory")
	}
	if !f.rejectDir("libs", "libs", "libs") {
		t.Fatal("must skip other trees")
	}
	if f.rejectFile("a.go", "services/payments/a.go") || !f.rejectFile("a.go", "libs/a.go") {
		t.Fatal("file scope")
	}
}

func TestWritePolicyEmptyListsStayDistinct(t *testing.T) {
	onlyInc := Filters{IncludeNames: []string{"a"}}
	onlyExc := Filters{ExcludeNames: []string{"a"}}
	if bytes.Equal([]byte(writePolicyBytes(onlyInc)), []byte(writePolicyBytes(onlyExc))) {
		t.Fatal("include vs exclude")
	}
}
