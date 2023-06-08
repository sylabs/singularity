package dirs

import (
	"os"
	"testing"
)

func MkdirOrFatal(t *testing.T, dir string, perm os.FileMode) {
	if err := os.Mkdir(dir, perm); err != nil {
		t.Fatalf("could not create %q: %s", dir, err)
	}
	if err := os.Chmod(dir, perm); err != nil {
		t.Fatalf("could not chmod %q to %o: %s", dir, perm, err)
	}
}
