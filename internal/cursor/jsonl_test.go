package cursor

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/geekmonkey/billy/internal/model"
)

func TestReadEvents_minFixture(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "cursor", "min.jsonl")
	ev, err := ReadEvents(root)
	if err != nil {
		t.Fatal(err)
	}
	var users, asst int
	for _, e := range ev {
		switch e.(type) {
		case model.EventUser:
			users++
		case model.EventAssistant:
			asst++
		}
	}
	if users != 1 || asst != 1 {
		t.Fatalf("users=%d asst=%d", users, asst)
	}
}
