package resources

import (
	"testing"
)

func TestParseLsOutput(t *testing.T) {
	output := `
total 8
drwxr-xr-x    1 root     root        4.0K 2025-05-30 12:13:44 +0000 beta
-rw-r--r--    1 root     root          12 2025-05-30 12:13:44 +0000 alpha
drwxr-xr-x    1 root     root        4.0K 2025-05-30 12:13:44 +0000 .
drwxr-xr-x    1 root     root        4.0K 2025-05-30 12:13:44 +0000 ..
ignored line
`

	files := parseLsOutput(output)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	if files[0].Name != "beta" || !files[0].IsDir {
		t.Fatalf("expected directory first, got %#v", files[0])
	}
	if files[1].Name != "alpha" || files[1].IsDir {
		t.Fatalf("expected file second, got %#v", files[1])
	}
}
