package agent

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/solo-ai/solo/pkg/skillloader"
)

// PrepareRevisionSkills publishes a verified, private snapshot through the
// provider's existing discovery roots. The caller holds the Agent execution lock.
// A single current symlink selects all Skill contents atomically; retry repairs
// any discovery links left by an interrupted preparation before starting a model.
func PrepareRevisionSkills(workDir, provider string, input []skillloader.Bundle) (string, error) {
	bundles, err := skillloader.NormalizeBundles(input)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(workDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	const base = ".solo/revision-skills"
	if err := root.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	oldTarget, err := root.Readlink(base + "/current")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("versioned Skill pointer: %w", err)
	}
	if oldTarget != "" && (!strings.HasPrefix(oldTarget, "snapshot-") || strings.ContainsAny(oldTarget, `/\`)) {
		return "", fmt.Errorf("invalid versioned Skill pointer")
	}
	// No files to manage on an existing, unversioned Agent.
	if len(bundles) == 0 && oldTarget == "" {
		return "", nil
	}
	var previous []skillloader.Bundle
	if oldTarget != "" {
		data, err := root.ReadFile(base + "/current/manifest.json")
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(data, &previous); err != nil {
			return "", err
		}
		if previous, err = skillloader.NormalizeBundles(previous); err != nil {
			return "", fmt.Errorf("invalid previous Skill manifest: %w", err)
		}
	}
	names := map[string]bool{}
	for _, bundle := range previous {
		names[bundle.Name] = false
	}
	for _, bundle := range bundles {
		names[bundle.Name] = true
	}
	// Include every supported provider root so switching provider also removes
	// stale versioned skills. Ordinary local and bundled skills remain untouched.
	roots := skillTargetRoots(workDir, "opencode")
	roots = append(roots, filepath.Join(workDir, ".codex", "skills"), filepath.Join(workDir, "skills"))
	wantedRoots := map[string]bool{}
	for _, target := range skillTargetRoots(workDir, provider) {
		wantedRoots[target] = true
	}
	type linkChange struct {
		name, target string
		keep         bool
	}
	var links []linkChange
	for _, targetRoot := range roots {
		relRoot, err := filepath.Rel(workDir, targetRoot)
		if err != nil {
			return "", err
		}
		if err := root.MkdirAll(relRoot, 0o700); err != nil {
			return "", err
		}
		for name, present := range names {
			dest := filepath.Join(relRoot, name)
			target, _ := filepath.Rel(relRoot, filepath.Join(base, "current", name))
			info, statErr := root.Lstat(dest)
			if statErr == nil {
				actual, linkErr := root.Readlink(dest)
				if info.Mode()&os.ModeSymlink == 0 || linkErr != nil || actual != target {
					return "", fmt.Errorf("Skill %s conflicts with an unmanaged local Skill at %s; rename the candidate Skill", name, dest)
				}
			} else if !errors.Is(statErr, fs.ErrNotExist) {
				return "", statErr
			}
			links = append(links, linkChange{dest, target, present && wantedRoots[targetRoot]})
		}
	}
	snapshot := "snapshot-" + rand.Text()
	stage := base + "/" + snapshot
	if err := root.Mkdir(stage, 0o700); err != nil {
		return "", err
	}
	switched := false
	defer func() {
		if !switched {
			_ = root.RemoveAll(stage)
		}
	}()
	for _, bundle := range bundles {
		for _, file := range bundle.Files {
			dest := filepath.Join(stage, bundle.Name, filepath.FromSlash(file.Path))
			if err := root.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				return "", err
			}
			mode := fs.FileMode(0o400)
			if file.Executable {
				mode = 0o500
			}
			if err := root.WriteFile(dest, file.Content, mode); err != nil {
				return "", err
			}
		}
	}
	manifest, err := json.Marshal(bundles)
	if err != nil {
		return "", err
	}
	if err := root.WriteFile(stage+"/manifest.json", manifest, 0o400); err != nil {
		return "", err
	}
	// The staging directory is new and was written through os.Root: no archive
	// extraction, symbolic-link traversal, or unchecked absolute source paths.
	pointer := base + "/next-" + rand.Text()
	if err := root.Symlink(snapshot, pointer); err != nil {
		return "", err
	}
	defer root.Remove(pointer)
	if err := root.Rename(pointer, base+"/current"); err != nil {
		return "", err
	}
	switched = true
	for _, link := range links {
		if !link.keep {
			if err := root.Remove(link.name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", err
			}
			continue
		}
		if err := root.Symlink(link.target, link.name); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	if oldTarget != "" {
		if err := root.RemoveAll(base + "/" + oldTarget); err != nil {
			return "", err
		}
	}
	return skillloader.BundlesDigest(bundles), nil
}
