package skillloader

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

const BundleCapability = "skill_bundle_v1"
const MaxBundleBytes = 4 << 20

// Bundle carries only explicitly selected reusable files, never an Agent workspace.
// []byte uses JSON base64 so scripts, images and other binary resources retain their bytes.
type Bundle struct {
	Name   string       `json:"name"`
	Files  []BundleFile `json:"files"`
	SHA256 string       `json:"sha256,omitempty"`
}

type BundleFile struct {
	Path       string `json:"path"`
	Content    []byte `json:"content"`
	Executable bool   `json:"executable,omitempty"`
}

var bundleName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

// ValidBundleName is shared by capability requirements and file package validation.
func ValidBundleName(name string) bool {
	return bundleName.MatchString(name) && !strings.HasPrefix(name, "solo-")
}

// NormalizeBundles validates both API uploads and daemon dispatches. A supplied
// digest is an assertion, not permission to skip checking the actual file bytes.
func NormalizeBundles(input []Bundle) ([]Bundle, error) {
	if len(input) > 20 {
		return nil, fmt.Errorf("at most 20 versioned Skills are allowed")
	}
	if len(input) == 0 {
		return nil, nil
	}
	result := make([]Bundle, len(input))
	names := map[string]bool{}
	total, count := 0, 0
	for i, item := range input {
		if !ValidBundleName(item.Name) || names[item.Name] {
			return nil, fmt.Errorf("invalid, reserved, or duplicate Skill name %q", item.Name)
		}
		names[item.Name] = true
		files := append([]BundleFile(nil), item.Files...)
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		paths := map[string]bool{}
		found := false
		for j, file := range files {
			files[j].Content = append([]byte{}, file.Content...)
			if !fs.ValidPath(file.Path) || file.Path == "." || len(file.Path) > 240 || strings.ContainsAny(file.Path, `\:`) {
				return nil, fmt.Errorf("invalid Skill file path %q", file.Path)
			}
			for _, part := range strings.Split(file.Path, "/") {
				lower := strings.ToLower(part)
				if strings.HasPrefix(part, ".") || lower == "memory.md" || lower == "memory" || lower == "credentials.json" || lower == "solo-config.json" || lower == "node_modules" {
					return nil, fmt.Errorf("private, hidden, or dependency file is not a reusable Skill: %q", file.Path)
				}
			}
			key := strings.ToLower(file.Path)
			if paths[key] {
				return nil, fmt.Errorf("duplicate Skill file %q", file.Path)
			}
			for dir := path.Dir(key); dir != "."; dir = path.Dir(dir) {
				if paths[dir] {
					return nil, fmt.Errorf("Skill file conflicts with directory %q", dir)
				}
			}
			paths[key] = true
			total += len(file.Content)
			count++
			if total > MaxBundleBytes || count > 500 {
				return nil, fmt.Errorf("Skill bundles exceed 4 MiB or 500 files")
			}
			if file.Path == "SKILL.md" {
				name, description, _, _, err := ParseFrontmatter(string(file.Content))
				if err != nil || name != item.Name || strings.TrimSpace(description) == "" {
					return nil, fmt.Errorf("SKILL.md must declare matching name and a description")
				}
				found = true
			}
		}
		for key := range paths {
			for dir := path.Dir(key); dir != "."; dir = path.Dir(dir) {
				if paths[dir] {
					return nil, fmt.Errorf("Skill file conflicts with directory %q", dir)
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("Skill %s needs a root SKILL.md", item.Name)
		}
		canonical := Bundle{Name: item.Name, Files: files}
		data, err := json.Marshal(canonical)
		if err != nil {
			return nil, err
		}
		canonical.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
		if item.SHA256 != "" && item.SHA256 != canonical.SHA256 {
			return nil, fmt.Errorf("Skill %s digest does not match its files", item.Name)
		}
		result[i] = canonical
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func BundlesDigest(bundles []Bundle) string {
	if len(bundles) == 0 {
		return ""
	}
	data, _ := json.Marshal(bundles)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
