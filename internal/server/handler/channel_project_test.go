package handler

import "testing"

func TestIsAbsoluteProjectPath(t *testing.T) {
	for _, path := range []string{"/Users/student/project", `C:\\Users\\student\\project`, `D:/project`, `\\server\share`} {
		if !isAbsoluteProjectPath(path) {
			t.Fatalf("expected absolute project path: %q", path)
		}
	}
	for _, path := range []string{"", ".", "project", "../project", `C:project`} {
		if isAbsoluteProjectPath(path) {
			t.Fatalf("expected relative project path: %q", path)
		}
	}
}
