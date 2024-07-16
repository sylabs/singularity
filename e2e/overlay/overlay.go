package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
	"github.com/sylabs/singularity/v4/e2e/internal/testhelper"
)

type ctx struct {
	env e2e.TestEnv
}

func (c ctx) testOverlayCreate(t *testing.T) {
	require.Filesystem(t, "overlay")
	require.MkfsExt3(t)
	e2e.EnsureImage(t, c.env)
	busyboxSIF := e2e.BusyboxSIF(t)

	tmpDir, cleanup := e2e.MakeTempDir(t, c.env.TestDir, "overlay", "")
	defer cleanup(t)

	pgpDir, _ := e2e.MakeSyPGPDir(t, tmpDir)
	c.env.KeyringDir = pgpDir

	sifSignedImage := filepath.Join(tmpDir, "signed.sif")
	sifImage := filepath.Join(tmpDir, "unsigned.sif")
	ext3SparseImage := filepath.Join(tmpDir, "image.sparse.ext3")
	ext3Image := filepath.Join(tmpDir, "image.ext3")
	ext3DirImage := filepath.Join(tmpDir, "imagedir.ext3")

	// signed SIF image
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(sifSignedImage, busyboxSIF),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("key import"),
		e2e.WithArgs("testdata/ecl-pgpkeys/key1.asc"),
		e2e.ConsoleRun(e2e.ConsoleSendLine("e2e")),
		e2e.ExpectExit(0),
	)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("sign"),
		e2e.WithArgs("-k", "0", sifSignedImage),
		e2e.ConsoleRun(e2e.ConsoleSendLine("e2e")),
		e2e.ExpectExit(0),
	)

	// unsigned SIF image
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("build"),
		e2e.WithArgs(sifImage, busyboxSIF),
		e2e.ExpectExit(0),
	)

	type test struct {
		name    string
		profile e2e.Profile
		command string
		args    []string
		exit    int
	}

	tests := []test{
		{
			name:    "create ext3 overlay with small size",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", "--size", "1", ext3Image},
			exit:    255,
		},
		{
			name:    "create ext3 sparse overlay image",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", "--size", "128", "--sparse", ext3SparseImage},
			exit:    0,
		},
		{
			name:    "create ext3 overlay image",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", "--size", "128", ext3Image},
			exit:    0,
		},
		{
			name:    "check ext3 overlay size",
			profile: e2e.UserProfile,
			command: "exec",
			args:    []string{"-B", ext3Image + ":/mnt/image", c.env.ImagePath, "/bin/sh", "-c", "[ $(stat -c %s /mnt/image) = 134217728 ] || false"},
			exit:    0,
		},
		{
			name:    "create ext3 overlay with an existing image",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", ext3Image},
			exit:    255,
		},
		{
			name:    "create ext3 overlay with dir",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", "--create-dir", "/usr/local/testing", ext3DirImage},
			exit:    0,
		},
		{
			name:    "check overlay dir permissions",
			profile: e2e.UserProfile,
			command: "exec",
			args:    []string{"-o", ext3DirImage, c.env.ImagePath, "mkdir", "/usr/local/testing/perms"},
			exit:    0,
		},
		{
			name:    "create ext3 overlay image in unsigned SIF",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", sifImage},
			exit:    0,
		},
		{
			name:    "create ext3 overlay image in SIF with an existing overlay",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", sifImage},
			exit:    255,
		},
		{
			name:    "create ext3 overlay image in signed SIF",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", sifSignedImage},
			exit:    255,
		},
	}

	err := e2e.CheckCryptsetupVersion()
	if err == nil {
		// encrypted SIF image
		passphraseEnvVar := fmt.Sprintf("%s=%s", "SINGULARITY_ENCRYPTION_PASSPHRASE", e2e.Passphrase)

		sifEncryptedImage := filepath.Join(tmpDir, "encrypted.sif")

		c.env.RunSingularity(
			t,
			e2e.WithProfile(e2e.RootProfile),
			e2e.WithCommand("build"),
			e2e.WithArgs("--encrypt", sifEncryptedImage, busyboxSIF),
			e2e.WithEnv(append(os.Environ(), passphraseEnvVar)),
			e2e.ExpectExit(0),
		)

		tests = append(tests, test{
			name:    "create ext3 overlay image in encrypted SIF",
			profile: e2e.RootProfile,
			command: "overlay",
			args:    []string{"create", sifEncryptedImage},
			exit:    255,
		})
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

func (c ctx) testOverlayOCI(t *testing.T) {
	require.Command(t, "fuse2fs")
	require.Command(t, "fuse-overlayfs")
	require.Command(t, "fusermount")
	require.MkfsExt3(t)
	e2e.EnsureOCISIF(t, c.env)

	tmpDir := t.TempDir()
	pgpDir, _ := e2e.MakeSyPGPDir(t, tmpDir)
	c.env.KeyringDir = pgpDir
	ocisifSigned := filepath.Join(tmpDir, "signed.sif")
	ocisif := filepath.Join(tmpDir, "unsigned.sif")

	// signed OCI-SIF image
	err := fs.CopyFile(c.env.OCISIFPath, ocisifSigned, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("key import"),
		e2e.WithArgs("testdata/ecl-pgpkeys/key1.asc"),
		e2e.ConsoleRun(e2e.ConsoleSendLine("e2e")),
		e2e.ExpectExit(0),
	)
	c.env.RunSingularity(
		t,
		e2e.WithProfile(e2e.UserProfile),
		e2e.WithCommand("sign"),
		e2e.WithArgs("-k", "0", ocisifSigned),
		e2e.ConsoleRun(e2e.ConsoleSendLine("e2e")),
		e2e.ExpectExit(0),
	)

	// unsigned SIF image
	err = fs.CopyFile(c.env.OCISIFPath, ocisif, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	type test struct {
		name    string
		profile e2e.Profile
		command string
		args    []string
		exit    int
	}

	// Size / sparse / directory additions in the ext3 image are checked by the
	// native SIF tests above. Same code path for OCI-SIF. We don't need to
	// repeat them here.
	tests := []test{
		{
			name:    "create fail signed",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", ocisifSigned},
			exit:    255,
		},
		{
			name:    "create",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", ocisif},
			exit:    0,
		},
		{
			name:    "create fail existing",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"create", ocisif},
			exit:    255,
		},
		// Add a file without `--writable` - should go into ephemeral tmpfs, not the overlay.
		{
			name:    "tmpfs touch",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{ocisif, "touch", "/in-overlay"},
			exit:    0,
		},
		{
			name:    "tmpfs check",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{ocisif, "ls", "/in-overlay"},
			exit:    1,
		},

		// Add a file to the overlay with `--writable` and check that it exists on re-run.
		{
			name:    "writable touch",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{"--writable", ocisif, "touch", "/in-overlay"},
			exit:    0,
		},
		{
			name:    "writable touch check",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{ocisif, "ls", "/in-overlay"},
			exit:    0,
		},
		// Synchronize the overlay digest.
		{
			name:    "sync",
			profile: e2e.UserProfile,
			command: "overlay",
			args:    []string{"sync", ocisif},
			exit:    0,
		},
		// Remove file without `--writable` - should be an ephemeral change, file still in overlay.
		{
			name:    "tmpfs rm",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{ocisif, "rm", "/in-overlay"},
			exit:    0,
		},
		{
			name:    "tmpfs rm check",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{ocisif, "ls", "/in-overlay"},
			exit:    0,
		},
		// Remove file with `--writable` - file gone from overlay.
		{
			name:    "writable rm",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{"--writable", ocisif, "rm", "/in-overlay"},
			exit:    0,
		},
		{
			name:    "writable rm check",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{ocisif, "ls", "/in-overlay"},
			exit:    1,
		},
		// Touch file without `--writable` and no tmpfs (via --no-compat)... should fail
		{
			name:    "readonly touch",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{"--no-compat", ocisif, "touch", "/in-overlay"},
			exit:    1,
		},
		{
			name:    "readonly touch check",
			profile: e2e.OCIUserProfile,
			command: "exec",
			args:    []string{ocisif, "ls", "/in-overlay"},
			exit:    1,
		},
		// Seal the overlay
		{
			name:    "seal",
			profile: e2e.OCIUserProfile,
			command: "overlay",
			args:    []string{"seal", ocisif},
			exit:    0,
		},
	}

	for _, tt := range tests {
		c.env.RunSingularity(
			t,
			e2e.AsSubtest(tt.name),
			e2e.WithProfile(tt.profile),
			e2e.WithCommand(tt.command),
			e2e.WithArgs(tt.args...),
			e2e.ExpectExit(tt.exit),
		)
	}
}

// E2ETests is the main func to trigger the test suite
func E2ETests(env e2e.TestEnv) testhelper.Tests {
	c := ctx{
		env: env,
	}

	return testhelper.Tests{
		"create": c.testOverlayCreate,
		"OCI":    c.testOverlayOCI,
	}
}
