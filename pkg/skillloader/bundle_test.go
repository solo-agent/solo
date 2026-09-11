package skillloader

import (
	"strings"
	"testing"
)

func TestNormalizeBundles(t *testing.T) {
	valid := func() []Bundle {
		return []Bundle{{Name: "calculate", Files: []BundleFile{{Path: "scripts/check.py", Content: []byte("print(42)"), Executable: true}, {Path: "SKILL.md", Content: []byte("---\nname: calculate\ndescription: Calculate a checked result\n---\nRun scripts/check.py")}}}}
	}
	canonical, err := NormalizeBundles(valid())
	if err != nil {
		t.Fatal(err)
	}
	again, err := NormalizeBundles(canonical)
	if err != nil || BundlesDigest(again) != BundlesDigest(canonical) {
		t.Fatalf("unstable canonical bundle: %v", err)
	}
	reordered := valid()
	reordered[0].Files[0], reordered[0].Files[1] = reordered[0].Files[1], reordered[0].Files[0]
	sorted, err := NormalizeBundles(reordered)
	if err != nil || sorted[0].SHA256 != canonical[0].SHA256 {
		t.Fatal("file order changed digest")
	}
	for _, test := range []struct {
		name   string
		change func([]Bundle)
	}{
		{"traversal", func(b []Bundle) { b[0].Files[0].Path = "../secret" }},
		{"absolute", func(b []Bundle) { b[0].Files[0].Path = "/secret" }},
		{"windows", func(b []Bundle) { b[0].Files[0].Path = `C:\secret` }},
		{"memory", func(b []Bundle) { b[0].Files[0].Path = "references/MEMORY.md" }},
		{"credentials", func(b []Bundle) { b[0].Files[0].Path = "credentials.json" }},
		{"hidden", func(b []Bundle) { b[0].Files[0].Path = ".env" }},
		{"missing frontmatter", func(b []Bundle) { b[0].Files[1].Content = []byte("# Skill") }},
		{"name mismatch", func(b []Bundle) { b[0].Name = "different" }},
		{"reserved", func(b []Bundle) { b[0].Name = "solo-send" }},
		{"digest mismatch", func(b []Bundle) { b[0].SHA256 = strings.Repeat("0", 64) }},
		{"case collision", func(b []Bundle) { b[0].Files = append(b[0].Files, BundleFile{Path: "skill.md"}) }},
		{"parent collision", func(b []Bundle) { b[0].Files = append(b[0].Files, BundleFile{Path: "SCRIPTS"}) }},
		{"size", func(b []Bundle) { b[0].Files[0].Content = make([]byte, MaxBundleBytes) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := valid()
			test.change(b)
			if _, err := NormalizeBundles(b); err == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
	b := valid()
	b[0].Files[0].Executable = false
	changed, _ := NormalizeBundles(b)
	if changed[0].SHA256 == canonical[0].SHA256 {
		t.Fatal("execution permission missing from digest")
	}
	if empty, err := NormalizeBundles(nil); err != nil || BundlesDigest(empty) != "" {
		t.Fatal("empty bundle breaks existing config")
	}
}
