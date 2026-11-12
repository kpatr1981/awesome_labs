package lab

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"awesome_labs/internal/lima"
)

// The image cache keeps a copy on the host of every container image a lab VM
// was built with, under <state>/cache/images/<runtime>/, one archive per
// image. A VM built from scratch loads them before its bootstrap runs, so the
// bootstrap's pulls find the images already there and a rebuild doesn't hit
// Docker Hub (and its pull rate limit) again. The archives are streamed over
// the VM's ssh connection, so the VM templates need no host mount.

// Runtimes: the Docker engine (docker, offsec) and the k8s nodes' containerd.
const (
	rtDocker     = "docker"
	rtContainerd = "containerd"
)

func runtimeOf(vm VM) string {
	if vm.Role == "cp" || vm.Role == "worker" {
		return rtContainerd
	}
	return rtDocker
}

// ImageCacheDir is where the archives of runtime rt live.
func (m *Manager) ImageCacheDir(rt string) string {
	return filepath.Join(m.StateDir, "cache", "images", rt)
}

// cacheFile maps an image reference to its archive name (and back, see refOf).
func cacheFile(ref string) string { return url.QueryEscape(ref) + ".tar" }

func refOf(file string) (string, bool) {
	if !strings.HasSuffix(file, ".tar") {
		return "", false
	}
	ref, err := url.QueryUnescape(strings.TrimSuffix(file, ".tar"))
	return ref, err == nil && ref != ""
}

// cachedRefs lists the images cached for runtime rt, sorted.
func (m *Manager) cachedRefs(rt string) []string {
	ents, _ := os.ReadDir(m.ImageCacheDir(rt))
	var refs []string
	for _, e := range ents {
		if ref, ok := refOf(e.Name()); ok && e.Type().IsRegular() {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}

// The in-VM commands, per runtime. $REF is the image reference.
var (
	listImagesCmd = map[string]string{
		rtDocker:     `docker images --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | sort -u`,
		rtContainerd: `ctr -n k8s.io images ls -q | grep -v -e '^sha256:' -e '@sha256:' | sort -u`,
	}
	saveImageCmd = map[string]string{
		rtDocker:     `docker save --platform "linux/$(dpkg --print-architecture)" "$REF"`,
		rtContainerd: `ctr -n k8s.io images export --platform "linux/$(dpkg --print-architecture)" - "$REF"`,
	}
	loadImageCmd = map[string]string{
		rtDocker:     `docker load -q >/dev/null`,
		rtContainerd: `ctr -n k8s.io images import --platform "linux/$(dpkg --print-architecture)" - >/dev/null`,
	}
)

// vmImages lists the tagged images vm has, as the VM names them, and the set
// of those names in both spellings (see shortRef).
func (m *Manager) vmImages(ctx context.Context, vm VM) ([]string, map[string]bool, error) {
	var out bytes.Buffer
	code, err := lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: listImagesCmd[runtimeOf(vm)]}, &out)
	if err == nil && code != 0 {
		err = fmt.Errorf("listing images on %s exited %d: %s", vm.Hostname, code, lastLine(out.String()))
	}
	if err != nil {
		return nil, nil, err
	}
	refs := strings.Fields(out.String())
	have := map[string]bool{}
	for _, ref := range refs {
		have[ref] = true
		// docker lists Hub images short (nginx:1.27-alpine), containerd in
		// full (docker.io/library/nginx:1.27-alpine); know both spellings.
		have[shortRef(ref)] = true
	}
	return refs, have, nil
}

// shortRef drops the docker.io/ and library/ prefixes containerd adds.
func shortRef(ref string) string {
	ref = strings.TrimPrefix(ref, "docker.io/")
	return strings.TrimPrefix(ref, "library/")
}

// loadCachedImages copies into vm every cached image it doesn't have yet. The
// k8s nodes also take the Docker cache, as most of their workload images are
// the docker host's too. Failures are logged and skipped: the bootstrap then
// pulls the image as before.
func (m *Manager) loadCachedImages(ctx context.Context, vm VM, log io.Writer) {
	rt := runtimeOf(vm)
	_, have, err := m.vmImages(ctx, vm)
	if err != nil {
		fmt.Fprintf(m.logFile, "image cache: %v\n", err)
		return
	}
	type item struct{ ref, path string }
	var todo []item
	var k8sImages map[string]bool
	dirs := []string{rt}
	if rt == rtContainerd {
		dirs = append(dirs, rtDocker)
		k8sImages = m.listedImages("k")
	}
	for _, d := range dirs {
		for _, ref := range m.cachedRefs(d) {
			if have[ref] || have[shortRef(ref)] {
				continue
			}
			if d != rt && !k8sImages[shortRef(ref)] {
				continue // a docker-only image would just take the node's disk
			}
			have[ref], have[shortRef(ref)] = true, true
			todo = append(todo, item{ref, filepath.Join(m.ImageCacheDir(d), cacheFile(ref))})
		}
	}
	if len(todo) == 0 {
		return
	}
	fmt.Fprintf(log, "==> [%s] loading %d images from the local cache\n", vm.Hostname, len(todo))
	for i, it := range todo {
		f, err := os.Open(it.path)
		if err != nil {
			continue
		}
		var out bytes.Buffer
		code, err := lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: loadImageCmd[rt], Stdin: f}, &out)
		f.Close()
		st := "loaded"
		if err != nil || code != 0 {
			st = "could not load"
			fmt.Fprintf(m.logFile, "image cache: load %s on %s: exit %d %v %s\n", it.ref, vm.Hostname, code, err, lastLine(out.String()))
		}
		fmt.Fprintf(log, "==> [%s] image %d/%d %s %s\n", vm.Hostname, i+1, len(todo), st, it.ref)
	}
}

// listedImages is the set of images vms/kit/images.txt pre-pulls where
// "where" holds tag (d = docker host, k = k8s nodes).
func (m *Manager) listedImages(tag string) map[string]bool {
	set := map[string]bool{}
	data, _ := fs.ReadFile(m.Assets, "vms/kit/images.txt")
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && !strings.HasPrefix(f[0], "#") && strings.Contains(f[1], tag) {
			set[f[0]] = true
		}
	}
	return set
}

// SaveImages copies to the host cache every image vm has that isn't cached
// yet, and returns how many it saved.
func (m *Manager) SaveImages(ctx context.Context, vm VM, log io.Writer) (int, error) {
	rt := runtimeOf(vm)
	all, _, err := m.vmImages(ctx, vm)
	if err != nil {
		return 0, err
	}
	dir := m.ImageCacheDir(rt)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	cached := map[string]bool{}
	for _, ref := range m.cachedRefs(rt) {
		cached[ref] = true
	}
	if rt == rtContainerd {
		for _, ref := range m.cachedRefs(rtDocker) {
			cached["docker.io/"+ref] = true // loaded from there on the next build
			cached["docker.io/library/"+ref] = true
			cached[ref] = true
		}
	}
	var refs []string
	for _, ref := range all {
		if !cached[ref] {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	saved := 0
	for i, ref := range refs {
		fmt.Fprintf(log, "==> [%s] caching image %d/%d %s\n", vm.Hostname, i+1, len(refs), ref)
		if err := m.saveImage(ctx, vm, ref, filepath.Join(dir, cacheFile(ref))); err != nil {
			fmt.Fprintf(log, "==> [%s] could not cache %s: %v\n", vm.Hostname, ref, err)
			continue
		}
		saved++
	}
	return saved, nil
}

func (m *Manager) saveImage(ctx context.Context, vm VM, ref, path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".partial-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	var stderr bytes.Buffer
	code, err := lima.Run(ctx, vm.Name, lima.Script{
		AsRoot: true, Body: saveImageCmd[runtimeOf(vm)], Env: map[string]string{"REF": ref},
		Stdin: strings.NewReader(""), Stderr: &stderr,
	}, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && code != 0 {
		err = fmt.Errorf("exit %d: %s", code, lastLine(stderr.String()))
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// saveEnvImages caches the images of every VM of env; failing to is not fatal.
func (m *Manager) saveEnvImages(ctx context.Context, env Env, log io.Writer) {
	for _, vm := range env.VMs {
		if n, err := m.SaveImages(ctx, vm, log); err != nil {
			fmt.Fprintf(log, "==> could not cache the images of %s: %v\n", vm.Name, err)
		} else if n > 0 {
			fmt.Fprintf(log, "==> [%s] cached %d images on this computer\n", vm.Hostname, n)
		}
	}
}

// CacheEnv brings env up and caches all its images on the host.
func (m *Manager) CacheEnv(ctx context.Context, env Env, log io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensure(ctx, env, log); err != nil {
		return err
	}
	m.saveEnvImages(ctx, env, log)
	return nil
}

// CacheSummary describes the image cache, one line per runtime.
func (m *Manager) CacheSummary() string {
	var b strings.Builder
	for _, rt := range []string{rtDocker, rtContainerd} {
		refs := m.cachedRefs(rt)
		var size int64
		for _, ref := range refs {
			if fi, err := os.Stat(filepath.Join(m.ImageCacheDir(rt), cacheFile(ref))); err == nil {
				size += fi.Size()
			}
		}
		fmt.Fprintf(&b, "%-11s %3d images  %6.1f GB  %s\n", rt, len(refs), float64(size)/1e9, m.ImageCacheDir(rt))
	}
	return b.String()
}
