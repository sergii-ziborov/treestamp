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

func TestWritePolicyEmptyListsStayDistinct(t *testing.T) {
	onlyInc := Filters{IncludeNames: []string{"a"}}
	onlyExc := Filters{ExcludeNames: []string{"a"}}
	if bytes.Equal([]byte(writePolicyBytes(onlyInc)), []byte(writePolicyBytes(onlyExc))) {
		t.Fatal("include vs exclude")
	}
}
