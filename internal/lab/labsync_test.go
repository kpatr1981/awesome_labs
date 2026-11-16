package lab

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dirRemote is a fake VM /lab backed by a local folder.
type dirRemote struct {
	root string
	down bool
}

func (d *dirRemote) list(context.Context) (map[string]entry, error) {
	if d.down {
		return nil, errUnavailable
	}
	res := map[string]entry{}
	err := filepath.WalkDir(d.root, func(p string, e fs.DirEntry, err error) error {
		if err != nil || p == d.root {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, d.root+"/"))
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		fi, _ := e.Info()
		switch {
		case e.IsDir():
			res[rel] = entry{dir: true, sig: "d"}
		case e.Type().IsRegular():
			res[rel] = entry{sig: fmt.Sprintf("%d %d %o", fi.ModTime().UnixNano(), fi.Size(), fi.Mode().Perm()), mode: fi.Mode().Perm()}
		}
		return nil
	})
	return res, err
}

func (d *dirRemote) pull(_ context.Context, paths []string) (map[string]pulled, error) {
	res := map[string]pulled{}
	for _, p := range paths {
		full := filepath.Join(d.root, p)
		b, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		fi, _ := os.Stat(full)
		res[p] = pulled{b, fi.Mode().Perm()}
	}
	return res, nil
}

func (d *dirRemote) push(_ context.Context, dirs []string, files []pushItem) error {
	for _, p := range dirs {
		if err := os.MkdirAll(filepath.Join(d.root, p), 0o755); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := writeInPlace(filepath.Join(d.root, f.path), f.data, f.mode); err != nil {
			return err
		}
	}
	return nil
}

func (d *dirRemote) remove(_ context.Context, paths []string) error {
	for _, p := range paths {
		if err := os.RemoveAll(filepath.Join(d.root, p)); err != nil {
			return err
		}
	}
	return nil
}

type syncFixture struct {
	t      *testing.T
	vm     *dirRemote
	s      *labSync
	local  string
	backup string
	log    *bytes.Buffer
	pushes int
}

func newSyncFixture(t *testing.T) *syncFixture {
	base := t.TempDir()
	f := &syncFixture{t: t, vm: &dirRemote{root: filepath.Join(base, "vm")}, local: filepath.Join(base, "local"),
		backup: filepath.Join(base, "backup"), log: &bytes.Buffer{}}
	for _, d := range []string{f.vm.root, f.local} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.s = newLabSync(f.local, f.backup, f.vm, f.log)
	f.s.onPush = func() { f.pushes++ }
	return f
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	// mtimes can repeat within a test: make each write visibly newer
	later := time.Now().Add(time.Duration(len(s)+1) * time.Millisecond)
	_ = os.Chtimes(p, later, later)
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func (f *syncFixture) round() {
	f.t.Helper()
	if err := f.s.round(context.Background()); err != nil {
		f.t.Fatal(err)
	}
}

func (f *syncFixture) vmFile(p string) string    { return filepath.Join(f.vm.root, p) }
func (f *syncFixture) localFile(p string) string { return filepath.Join(f.local, p) }

func backups(t *testing.T, root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			out = append(out, filepath.Base(p)+"="+read(t, p))
		}
		return nil
	})
	return out
}

func TestSyncMirrorsTheVMOnConnectAndKeepsWhatItReplaces(t *testing.T) {
	f := newSyncFixture(t)
	write(t, f.vmFile("TASK.md"), "task")
	write(t, f.vmFile("app/main.py"), "print(1)")
	write(t, f.vmFile("repo/.git/HEAD"), "ref")
	write(t, f.localFile("stale.txt"), "old exercise")
	write(t, f.localFile("app/main.py"), "my edit")
	f.round()
	if got := read(t, f.localFile("TASK.md")); got != "task" {
		t.Errorf("TASK.md = %q", got)
	}
	if got := read(t, f.localFile("app/main.py")); got != "print(1)" {
		t.Errorf("app/main.py = %q, want the VM's", got)
	}
	if got := read(t, f.localFile("stale.txt")); got != "<missing>" {
		t.Errorf("stale.txt survived the mirror: %q", got)
	}
	if got := read(t, f.localFile("repo/.git/HEAD")); got != "<missing>" {
		t.Errorf(".git was synced: %q", got)
	}
	got := strings.Join(backups(t, f.backup), ",")
	if !strings.Contains(got, "stale.txt=old exercise") || !strings.Contains(got, "main.py=my edit") {
		t.Errorf("backups = %q, want stale.txt and main.py kept", got)
	}
	if f.pushes != 0 {
		t.Errorf("the mirror wrote to the VM")
	}
}

func TestSyncBothWays(t *testing.T) {
	f := newSyncFixture(t)
	write(t, f.vmFile("a.txt"), "one")
	write(t, f.vmFile("dir/b.txt"), "two")
	f.round()

	// an edit here goes to the VM, and doesn't come back
	write(t, f.localFile("a.txt"), "one, edited")
	f.round()
	if got := read(t, f.vmFile("a.txt")); got != "one, edited" {
		t.Fatalf("VM a.txt = %q", got)
	}
	if f.pushes != 1 {
		t.Errorf("pushes = %d", f.pushes)
	}
	f.log.Reset()
	f.round()
	f.round()
	if f.log.Len() != 0 {
		t.Errorf("idle rounds did something: %q", f.log.String())
	}

	// an edit in the VM comes here
	write(t, f.vmFile("dir/b.txt"), "two, from the vm")
	f.round()
	if got := read(t, f.localFile("dir/b.txt")); got != "two, from the vm" {
		t.Errorf("local b.txt = %q", got)
	}

	// new files and folders here go to the VM
	write(t, f.localFile("new/deep/c.sh"), "#!/bin/sh")
	f.round()
	if got := read(t, f.vmFile("new/deep/c.sh")); got != "#!/bin/sh" {
		t.Errorf("VM c.sh = %q", got)
	}

	// deleting here deletes in the VM
	if err := os.RemoveAll(f.localFile("new")); err != nil {
		t.Fatal(err)
	}
	f.round()
	if _, err := os.Stat(f.vmFile("new")); err == nil {
		t.Error("new/ still in the VM")
	}

	// a reset empties /lab: the local copy follows
	_ = os.RemoveAll(f.vm.root)
	_ = os.MkdirAll(f.vm.root, 0o755)
	write(t, f.vmFile("TASK.md"), "next exercise")
	f.round()
	if got := read(t, f.localFile("a.txt")); got != "<missing>" {
		t.Errorf("a.txt survived the reset: %q", got)
	}
	if got := read(t, f.localFile("TASK.md")); got != "next exercise" {
		t.Errorf("TASK.md = %q", got)
	}
	if b := backups(t, f.backup); len(b) != 0 {
		t.Errorf("unchanged files were backed up: %v", b)
	}
}

func TestSyncConflictKeepsTheVMAndBacksUpTheLocalEdit(t *testing.T) {
	f := newSyncFixture(t)
	write(t, f.vmFile("x.conf"), "base")
	f.round()
	write(t, f.vmFile("x.conf"), "vm side")
	write(t, f.localFile("x.conf"), "local side!")
	f.round()
	if got := read(t, f.localFile("x.conf")); got != "vm side" {
		t.Errorf("local x.conf = %q, want the VM's", got)
	}
	if got := read(t, f.vmFile("x.conf")); got != "vm side" {
		t.Errorf("VM x.conf = %q", got)
	}
	if b := strings.Join(backups(t, f.backup), ","); b != "x.conf=local side!" {
		t.Errorf("backups = %q", b)
	}
}

func TestSyncReconnectMirrorsAgain(t *testing.T) {
	f := newSyncFixture(t)
	write(t, f.vmFile("a.txt"), "one")
	f.round()
	f.vm.down = true
	if err := f.s.round(context.Background()); err != errUnavailable {
		t.Fatalf("round with the VM down: %v", err)
	}
	f.s.first = true // what SyncLab does
	write(t, f.localFile("offline.txt"), "written while the VM was down")
	f.vm.down = false
	f.round()
	if _, err := os.Stat(f.vmFile("offline.txt")); err == nil {
		t.Error("a file from while the VM was away was pushed into the new lab")
	}
	if b := strings.Join(backups(t, f.backup), ","); !strings.Contains(b, "offline.txt=") {
		t.Errorf("offline.txt not kept: %q", b)
	}
}

func TestSyncNeverWritesThroughALocalSymlink(t *testing.T) {
	f := newSyncFixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, f.localFile("link")); err != nil {
		t.Fatal(err)
	}
	f.s.first = false
	write(t, f.vmFile("link/evil.txt"), "x")
	f.round()
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Error("a pull wrote through a symlink out of the folder")
	}
}
