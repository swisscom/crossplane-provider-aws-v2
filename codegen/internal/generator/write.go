/*
Copyright 2026 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generator

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/imports"
)

// unkeyedTime matches the unkeyed metav1.Time literals ACK emits.
var unkeyedTime = regexp.MustCompile(`metav1\.Time\{([^}:]+)\}`)

func format(path string, src []byte) ([]byte, error) {
	src = unkeyedTime.ReplaceAll(src, []byte("metav1.Time{Time: $1}"))
	return imports.Process(path, src, nil)
}

// formatAll formats every file (paths relative to root) in memory. If any
// fails, it writes their sources to <path>.broken for debugging and nothing
// else.
func formatAll(root string, files map[string][]byte) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files))
	var errs []error
	for _, p := range slices.Sorted(maps.Keys(files)) {
		b, err := format(filepath.Join(root, p), files[p])
		if err != nil {
			if werr := writeAtomic(filepath.Join(root, p)+".broken", files[p]); werr != nil {
				err = errors.Join(err, werr)
			}
			errs = append(errs, err)
			continue
		}
		out[p] = b
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("cannot format generated code (see *.broken files): %w", errors.Join(errs...))
	}
	return out, nil
}

// writeFiles writes the files in a stable order, each atomically, and
// removes .broken files a previous failed run left behind.
func writeFiles(root string, files map[string][]byte) error {
	for _, p := range slices.Sorted(maps.Keys(files)) {
		path := filepath.Join(root, p)
		if err := writeAtomic(path, files[p]); err != nil {
			return err
		}
		if err := os.Remove(path + ".broken"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// writeAtomic replaces path with b, so readers never see a partial file.
func writeAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}

// writeGo formats and writes a single file.
func writeGo(path string, src []byte) error {
	out, err := format(path, src)
	if err != nil {
		return fmt.Errorf("cannot format %s: %w", path, err)
	}
	return writeAtomic(path, out)
}

// staleFiles returns the generated files of a service (paths relative to
// root) that this run no longer renders, e.g. of a removed kind. A
// controller directory that also holds hand-written code is an error,
// because deleting only its zz_ files would leave code that can't compile.
func staleFiles(root, svc string, rendered map[string][]byte) ([]string, error) {
	var candidates []string
	for _, pattern := range []string{
		filepath.Join("apis", svc, "*", "zz_*.go"),
		filepath.Join("internal", "controller", svc, "*", "zz_*.go"),
	} {
		m, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, m...)
	}
	var stale []string
	dirs := map[string]bool{}
	for _, abs := range candidates {
		p, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, err
		}
		// controller-gen and angryjet own zz_generated.* (make generate).
		if _, ok := rendered[p]; ok || strings.HasPrefix(filepath.Base(p), "zz_generated.") {
			continue
		}
		stale = append(stale, p)
		if strings.HasPrefix(p, filepath.Join("internal", "controller")) {
			dirs[filepath.Dir(p)] = true
		}
	}
	var errs []error
	for _, d := range slices.Sorted(maps.Keys(dirs)) {
		hand, err := handWrittenGo(filepath.Join(root, d))
		if err != nil {
			return nil, err
		}
		if len(hand) > 0 {
			errs = append(errs, fmt.Errorf("%s: kind is no longer generated but has hand-written files (%s); delete the directory or stop ignoring the kind", filepath.ToSlash(d), strings.Join(hand, ", ")))
		}
	}
	slices.Sort(stale)
	return stale, errors.Join(errs...)
}

func handWrittenGo(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasPrefix(e.Name(), "zz_") {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// removeStale removes stale files, then their directories once empty.
func removeStale(root string, stale []string) error {
	dirs := map[string]bool{}
	for _, p := range stale {
		if err := os.Remove(filepath.Join(root, p)); err != nil && !os.IsNotExist(err) {
			return err
		}
		dirs[filepath.Join(root, filepath.Dir(p))] = true
	}
	for _, d := range slices.Sorted(maps.Keys(dirs)) {
		if entries, err := os.ReadDir(d); err == nil && len(entries) == 0 {
			if err := os.Remove(d); err != nil {
				return err
			}
		}
	}
	return nil
}
