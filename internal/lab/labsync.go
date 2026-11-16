package lab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A labSync keeps a local folder and a VM's /lab the same, both ways, by
// polling: each round lists both sides and compares them with what the last
// round left in sync. Whenever it (re)connects, the VM wins: anything of the
// user's it would overwrite or delete is copied to a backup folder first.

// maxSyncSize is the largest file mirrored; bigger ones stay where they are.
const maxSyncSize = 16 << 20

// errUnavailable marks a round that could not reach the VM's /lab.
var errUnavailable = errors.New("lab unavailable")

type entry struct {
	dir  bool
	sig  string // change signature; "d" for directories
	mode fs.FileMode
}

type pulled struct {
	data []byte
	mode fs.FileMode
}

type pushItem struct {
	path string
	data []byte
	mode fs.FileMode
}

// remote is the VM side of a sync. Paths are slash-separated, relative to /lab.
type remote interface {
	list(ctx context.Context) (map[string]entry, error) // errUnavailable when /lab can't be read
	pull(ctx context.Context, paths []string) (map[string]pulled, error)
	push(ctx context.Context, dirs []string, files []pushItem) error
	remove(ctx context.Context, paths []string) error
}

type synced struct{ r, l string } // r == "": pushed, the VM's signature not seen yet

type labSync struct {
	root   string // local folder
	backup string // where replaced local files are kept
	r      remote
	out    io.Writer
	onPush func() // called when anything is written to the VM
	now    func() time.Time

	state map[string]synced
	first bool   // the next round mirrors the VM
	stamp string // backup folder of the current round
}

func newLabSync(root, backup string, r remote, out io.Writer) *labSync {
	if out == nil {
		out = io.Discard
	}
	return &labSync{root: root, backup: backup, r: r, out: out, now: time.Now, state: map[string]synced{}, first: true}
}

func localSig(fi fs.FileInfo) string {
	return fmt.Sprintf("%d %d", fi.ModTime().UnixNano(), fi.Size())
}

// listLocal walks the local folder, skipping .git, symlinks and big files.
func (s *labSync) listLocal() (map[string]entry, error) {
	res := map[string]entry{}
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == s.root {
				return err
			}
			return nil
		}
		if p == s.root {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, s.root+string(filepath.Separator)))
		name := d.Name()
		switch {
		case d.IsDir() && name == ".git":
			return filepath.SkipDir
		case d.IsDir():
			res[rel] = entry{dir: true, sig: "d"}
		case d.Type().IsRegular() && name != ".DS_Store":
			fi, err := d.Info()
			if err != nil || fi.Size() > maxSyncSize {
				return nil
			}
			res[rel] = entry{sig: localSig(fi), mode: fi.Mode().Perm()}
		}
		return nil
	})
	return res, err
}

// safe reports whether p stays inside the local folder: relative, no "..",
// and no symlink along the way.
func (s *labSync) safe(p string) bool {
	if !filepath.IsLocal(filepath.FromSlash(p)) {
		return false
	}
	cur := s.root
	for _, part := range strings.Split(p, "/") {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return true // the rest doesn't exist yet
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

func (s *labSync) local(p string) string { return filepath.Join(s.root, filepath.FromSlash(p)) }

// dirty reports whether the local file p has changes the VM hasn't seen.
func (s *labSync) dirty(p string) bool {
	if s.first {
		return true
	}
	st, ok := s.state[p]
	if !ok {
		return true
	}
	fi, err := os.Stat(s.local(p))
	return err != nil || localSig(fi) != st.l
}

// keep copies the local file p into this round's backup folder.
func (s *labSync) keep(p string) {
	b, err := os.ReadFile(s.local(p))
	if err != nil {
		return
	}
	if s.stamp == "" {
		s.stamp = filepath.Join(s.backup, s.now().Format("20060102-150405"))
	}
	dst := filepath.Join(s.stamp, filepath.FromSlash(p))
	if os.MkdirAll(filepath.Dir(dst), 0o755) == nil && os.WriteFile(dst, b, 0o644) == nil {
		fmt.Fprintf(s.out, "  kept your copy of %s in %s\n", p, dst)
	}
}

// removeLocal deletes p (a file or a whole folder), keeping copies of any
// file with changes the VM hasn't seen.
func (s *labSync) removeLocal(p string) {
	full := s.local(p)
	_ = filepath.WalkDir(full, func(q string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			rel := filepath.ToSlash(strings.TrimPrefix(q, s.root+string(filepath.Separator)))
			if s.dirty(rel) {
				s.keep(rel)
			}
		}
		return nil
	})
	_ = os.RemoveAll(full)
	s.forget(p)
}

// forget drops p and everything below it from the sync state.
func (s *labSync) forget(p string) {
	for q := range s.state {
		if q == p || strings.HasPrefix(q, p+"/") {
			delete(s.state, q)
		}
	}
}

// topmost drops the paths inside another path of the sorted list: removing
// the folder removes them.
func topmost(paths []string) []string {
	var out []string
	for _, p := range paths {
		if !under(p, out) {
			out = append(out, p)
		}
	}
	return out
}

func under(p string, dirs []string) bool {
	for _, d := range dirs {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

// round runs one comparison and applies what it finds.
func (s *labSync) round(ctx context.Context) error {
	R, err := s.r.list(ctx)
	if err != nil {
		return err
	}
	L, err := s.listLocal()
	if err != nil {
		return err
	}
	mirror := s.first
	if mirror {
		s.state = map[string]synced{}
	}
	s.stamp = ""

	all := map[string]bool{}
	for p := range R {
		all[p] = true
	}
	for p := range L {
		all[p] = true
	}
	paths := make([]string, 0, len(all))
	for p := range all {
		paths = append(paths, p)
	}
	sort.Strings(paths) // parents before children

	for p, st := range s.state { // first sight of files we pushed
		if r, ok := R[p]; ok && st.r == "" && !r.dir {
			st.r = r.sig
			s.state[p] = st
		}
	}
	var pull, mkLocal, rmLocal, rmRemote, mkRemote []string
	var push []pushItem
	remoteChanged := func(p string) bool {
		r, ok := R[p]
		st, sok := s.state[p]
		return ok && (!sok || (st.r != "" && r.sig != st.r))
	}
	for _, p := range paths {
		r, rok := R[p]
		l, lok := L[p]
		st, sok := s.state[p]
		rch := remoteChanged(p)
		lch := lok && (!sok || l.sig != st.l)
		switch {
		case rok && lok && r.dir != l.dir: // a file on one side, a folder on the other: the VM wins
			rmLocal = append(rmLocal, p)
			if r.dir {
				mkLocal = append(mkLocal, p)
			} else {
				pull = append(pull, p)
			}
		case rok && lok && r.dir:
			s.state[p] = synced{"d", "d"}
		case rok && lok:
			switch {
			case !sok, rch: // new on both sides, changed in the VM, or both: the VM wins
				pull = append(pull, p)
			case lch:
				push = append(push, pushItem{path: p, mode: l.mode})
			}
		case rok: // only in the VM
			if sok && !rch && !mirror { // deleted here
				if r.dir {
					keepDir := false
					for q := range R {
						if strings.HasPrefix(q, p+"/") && (remoteChanged(q) || s.state[q].r == "") {
							keepDir = true
						}
					}
					if keepDir { // something in it changed in the VM: bring it back instead
						delete(s.state, p)
						mkLocal = append(mkLocal, p)
						continue
					}
				}
				rmRemote = append(rmRemote, p)
			} else if r.dir {
				mkLocal = append(mkLocal, p)
			} else {
				pull = append(pull, p)
			}
		case lok: // only here
			if sok || mirror { // deleted in the VM (a reset, a new exercise…), or not there on (re)connect
				rmLocal = append(rmLocal, p)
			} else if l.dir {
				mkRemote = append(mkRemote, p)
			} else {
				push = append(push, pushItem{path: p, mode: l.mode})
			}
		default:
			delete(s.state, p)
		}
	}
	rmRemote, rmLocal = topmost(rmRemote), topmost(rmLocal)
	// nothing goes into a folder that is being removed
	keepPush := push[:0]
	for _, it := range push {
		if !under(it.path, rmLocal) && !under(it.path, rmRemote) {
			keepPush = append(keepPush, it)
		}
	}
	push = keepPush
	var keepMk []string
	for _, d := range mkRemote {
		if !under(d, rmLocal) {
			keepMk = append(keepMk, d)
		}
	}
	mkRemote = keepMk

	var errs []error
	// the VM side
	if len(rmRemote) > 0 {
		if err := s.r.remove(ctx, rmRemote); err != nil {
			errs = append(errs, err)
		} else {
			for _, p := range rmRemote {
				s.forget(p)
				fmt.Fprintf(s.out, "✗ %s (deleted here, removed from the VM)\n", p)
			}
		}
	}
	for i := range push {
		b, err := os.ReadFile(s.local(push[i].path))
		if err != nil {
			continue
		}
		push[i].data = b
	}
	if len(push) > 0 || len(mkRemote) > 0 {
		if err := s.r.push(ctx, mkRemote, push); err != nil {
			errs = append(errs, err)
		} else {
			for _, d := range mkRemote {
				s.state[d] = synced{"d", "d"}
			}
			for _, it := range push {
				if fi, err := os.Stat(s.local(it.path)); err == nil {
					s.state[it.path] = synced{"", localSig(fi)}
				}
				fmt.Fprintf(s.out, "↑ %s\n", it.path)
			}
		}
		if s.onPush != nil {
			s.onPush()
		}
	} else if len(rmRemote) > 0 && s.onPush != nil {
		s.onPush()
	}
	// the local side
	for _, p := range rmLocal {
		if !s.safe(p) {
			continue
		}
		if _, err := os.Lstat(s.local(p)); err == nil {
			s.removeLocal(p)
			if !mirror {
				fmt.Fprintf(s.out, "✗ %s (deleted in the VM)\n", p)
			}
		}
	}
	for _, d := range mkLocal {
		if s.safe(d) && os.MkdirAll(s.local(d), 0o755) == nil {
			s.state[d] = synced{"d", "d"}
		}
	}
	if len(pull) > 0 {
		got, err := s.r.pull(ctx, pull)
		if err != nil {
			errs = append(errs, err)
		}
		for _, p := range pull {
			f, ok := got[p]
			if !ok || !s.safe(p) {
				continue
			}
			full := s.local(p)
			if old, err := os.ReadFile(full); err == nil && !bytes.Equal(old, f.data) && s.dirty(p) {
				s.keep(p)
			}
			if err := writeInPlace(full, f.data, f.mode.Perm()|0o600); err != nil {
				errs = append(errs, err)
				continue
			}
			if fi, err := os.Stat(full); err == nil {
				s.state[p] = synced{R[p].sig, localSig(fi)}
			}
			if !mirror {
				fmt.Fprintf(s.out, "↓ %s\n", p)
			}
		}
	}
	if mirror && len(errs) == 0 {
		s.first = false
		n := 0
		for _, e := range R {
			if !e.dir {
				n++
			}
		}
		fmt.Fprintf(s.out, "in sync: %d files\n", n)
	}
	return errors.Join(errs...)
}

// writeInPlace rewrites an existing file without replacing it (editors and
// watchers keep their handle), or creates it with its folders.
func writeInPlace(p string, data []byte, mode fs.FileMode) error {
	if fi, err := os.Lstat(p); err == nil && !fi.Mode().IsRegular() {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(p, mode)
}
