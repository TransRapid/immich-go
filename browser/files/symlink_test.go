package files

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/kr/pretty"
	"github.com/simulot/immich-go/immich"
	"github.com/simulot/immich-go/logger"
)

func TestSymlink(t *testing.T) {
	tmp, err := os.MkdirTemp("", "symtest")
	if err != nil {
		t.Error(err)
		return
	}

	defer func() {
		os.RemoveAll(tmp)
	}()

	err = os.Mkdir(filepath.Join(tmp, "branchA"), 0o770)
	if err != nil {
		t.Error(err)
		return
	}

	err = os.Mkdir(filepath.Join(tmp, "branchA", "subBranchA"), 0o770)
	if err != nil {
		t.Error(err)
		return
	}

	err = os.WriteFile(filepath.Join(tmp, "branchA", "subBranchA", "fileA.jpg"), []byte(filepath.Join(tmp, "branchA", "subBranchA", "fileA.jpg")), 0o777)
	if err != nil {
		t.Error(err)
		return
	}
	err = os.Mkdir(filepath.Join(tmp, "branchB"), 0o770)
	if err != nil {
		t.Error(err)
		return
	}

	err = os.Symlink(filepath.Join(tmp, "branchA", "subBranchA"), filepath.Join(tmp, "branchB", "subBranchB"))
	if err != nil {
		t.Error(err)
		return
	}

	fsys := os.DirFS(filepath.Join(tmp, "branchB"))
	ctx := context.Background()
	l := log.New(io.Discard)
	cnt := logger.NewCounters[logger.UpLdAction]()
	lc := logger.NewLogAndCount[logger.UpLdAction](l, logger.SendNop, cnt)
	b, err := NewLocalFiles(ctx, lc, fsys)
	if err != nil {
		t.Error(err)
	}
	b.SetSupportedMedia(immich.DefaultSupportedMedia)
	b.SetWhenNoDate("FILE")

	results := []string{}
	for a := range b.Browse(ctx) {
		results = append(results, a.FileName)
	}

	expected := []string{filepath.Join(tmp, "branchB", "fileA.jpg")}
	if !reflect.DeepEqual(results, expected) {
		t.Errorf("difference\n")
		pretty.Ldiff(t, expected, results)
	}
}
