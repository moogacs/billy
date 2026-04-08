package anthropic

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/geekmonkey/billy/internal/model"
)

func TestReadEvents_minFixture(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "anthropic", "min.jsonl")
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
	if users != 2 {
		t.Fatalf("users: got %d want 2", users)
	}
	if asst != 2 {
		t.Fatalf("assistant: got %d want 2 (dup message id dropped)", asst)
	}
}
