package filesystem

import "testing"

func TestAbsoluteSymlinkTranslation(t *testing.T) {
	if got := mountSymlinkTarget("/mnt/local", "/Users/test/code"); got != "/mnt/local/Users/test/code" {
		t.Fatalf("unexpected mount symlink target %q", got)
	}
	if got := localSymlinkTarget("/mnt/local", "/mnt/local/Users/test/code"); got != "/Users/test/code" {
		t.Fatalf("unexpected local symlink target %q", got)
	}
	if got := mountSymlinkTarget("/mnt/local", "../relative"); got != "../relative" {
		t.Fatalf("relative symlink target changed to %q", got)
	}
}
