// Copyright (c) 2021-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.package singularity

package singularity

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/internal/pkg/ocisif"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/image"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sys/unix"
)

const (
	mkfsBinary     = "mkfs.ext3"
	ddBinary       = "dd"
	truncateBinary = "truncate"
)

var (
	errOverlayEncrypted = errors.New("cannot add overlay to an encrypted image")
	errOverlaySigned    = errors.New("cannot add overlay to a signed image")
	errOverlayExists    = errors.New("writable overlay already exists in image")
	errOverlayEXT3      = errors.New("image is an EXT3 filesystem that can be used as an overlay directly")
	errOverlayNotSIF    = errors.New("cannot add an overlay to a non-SIF image")
)

// sifIsSigned returns true if the SIF in rw contains one or more signature objects.
func sifIsSigned(rw sif.ReadWriter) (bool, error) {
	f, err := sif.LoadContainer(rw,
		sif.OptLoadWithFlag(os.O_RDONLY),
		sif.OptLoadWithCloseOnUnload(false),
	)
	if err != nil {
		return false, err
	}
	defer f.UnloadContainer()

	sigs, err := f.GetDescriptors(sif.WithDataType(sif.DataSignature))
	return len(sigs) > 0, err
}

// canAddOverlay checks whether img supports having an overlay added. Err is set
// where it cannot.
func canAddOverlay(img *image.Image) (bool, error) {
	switch img.Type {
	case image.SIF:
		e, err := img.HasEncryptedRootFs()
		if err != nil {
			return false, fmt.Errorf("while checking for encryption: %s", err)
		}
		if e {
			return false, errOverlayEncrypted
		}

		signed, err := sifIsSigned(img.File)
		if err != nil {
			return false, fmt.Errorf("while checking for signatures: %s", err)
		} else if signed {
			return false, errOverlaySigned
		}

		overlays, err := img.GetOverlayPartitions()
		if err != nil {
			return false, fmt.Errorf("while getting SIF overlay partitions: %s", err)
		}
		for _, overlay := range overlays {
			if overlay.Type != image.EXT3 {
				continue
			}
			sylog.Infof("Existing overlay partition can be deleted with: singularity sif del %d %s", overlay.ID, img.Path)
			return false, errOverlayExists
		}

	case image.OCISIF:
		signed, err := sifIsSigned(img.File)
		if err != nil {
			return false, fmt.Errorf("while checking for signatures: %s", err)
		} else if signed {
			return false, errOverlaySigned
		}

		hasOverlay, _, err := ocisif.HasOverlay(img.Path)
		if err != nil {
			return false, fmt.Errorf("while checking for overlays: %s", err)
		} else if hasOverlay {
			return false, errOverlayExists
		}

	case image.EXT3:
		return false, errOverlayEXT3
	default:
		return false, errOverlayNotSIF
	}

	return true, nil
}

// addOverlayToSIF adds the EXT3 overlay at overlayPath to the SIF image at imagePath.
func addOverlayToSIF(imagePath, overlayPath string) error {
	f, err := sif.LoadContainerFromPath(imagePath)
	if err != nil {
		return err
	}
	defer f.UnloadContainer()

	tf, err := os.Open(overlayPath)
	if err != nil {
		return err
	}
	defer tf.Close()

	arch := f.PrimaryArch()
	if arch == "unknown" {
		arch = runtime.GOARCH
	}

	di, err := sif.NewDescriptorInput(sif.DataPartition, tf,
		sif.OptPartitionMetadata(sif.FsExt3, sif.PartOverlay, arch),
	)
	if err != nil {
		return err
	}

	return f.AddObject(di)
}

// findConvertCommand finds dd unless overlaySparse is true
func findConvertCommand(overlaySparse bool) (string, error) {
	// We can support additional arguments, so return a list
	command := ""

	// Sparse overlay requires truncate -s
	if overlaySparse {
		truncate, err := bin.FindBin(truncateBinary)
		if err != nil {
			return command, err
		}
		command = truncate

		// Regular (non sparse) requires dd
	} else {
		dd, err := bin.FindBin(ddBinary)
		if err != nil {
			return command, err
		}
		command = dd
	}
	return command, nil
}

// checkMkfsSupport checks if the mkfs binary support features required for overlay creation.
func checkMkfsSupport(mkfs string) error {
	// check if -d option is available
	buf := new(bytes.Buffer)
	cmd := exec.Command(mkfs, "--help")
	cmd.Stderr = buf
	// ignore error because the command always returns with exit code 1
	_ = cmd.Run()

	if !strings.Contains(buf.String(), "[-d ") {
		return fmt.Errorf("%s seems too old as it doesn't support -d, this is required to create the overlay layout", mkfsBinary)
	}

	return nil
}

// createOverlayFile creates a file holding an ext3 fs of specified size (MiB)
// and sparseness, suitable for use as a writable overlay. The file is created
// at path. Any directories listed in overlayDirs will be created in the overlay
// filesystem.
func createOverlayFile(path string, size int, sparse bool, overlayDirs ...string) error {
	mkfs, err := bin.FindBin(mkfsBinary)
	if err != nil {
		return err
	}
	if err := checkMkfsSupport(mkfs); err != nil {
		return err
	}

	// This can be dd or truncate (if supported and --sparse is true)
	convertCommand, err := findConvertCommand(sparse)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	if strings.Contains(convertCommand, "truncate") {
		cmd = exec.Command(convertCommand, fmt.Sprintf("--size=%dM", size), path)
	} else {
		cmd = exec.Command(convertCommand, "if=/dev/zero", "of="+path, "bs=1M", fmt.Sprintf("count=%d", size))
	}

	errBuf := new(bytes.Buffer)
	cmd.Stderr = errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("while zero'ing overlay image %s: %s\nCommand error: %s", path, err, errBuf)
	}
	errBuf.Reset()

	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("while setting 0600 permission on %s: %s", path, err)
	}

	tmpDir, err := os.MkdirTemp("", "overlay-")
	if err != nil {
		return fmt.Errorf("while creating temporary overlay directory: %s", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	perm := os.FileMode(0o755)

	if os.Getuid() > 65535 || os.Getgid() > 65535 {
		perm = 0o777
	}

	upperDir := filepath.Join(tmpDir, "upper")
	workDir := filepath.Join(tmpDir, "work")

	oldumask := unix.Umask(0)
	defer unix.Umask(oldumask)

	if err := os.Mkdir(upperDir, perm); err != nil {
		return fmt.Errorf("while creating %s: %s", upperDir, err)
	}
	if err := os.Mkdir(workDir, perm); err != nil {
		return fmt.Errorf("while creating %s: %s", workDir, err)
	}

	for _, dir := range overlayDirs {
		od := filepath.Join(upperDir, dir)
		if !strings.HasPrefix(od, upperDir) {
			return fmt.Errorf("overlay directory created outside of overlay layout %s", upperDir)
		}
		if err := os.MkdirAll(od, perm); err != nil {
			return fmt.Errorf("while creating %s: %s", od, err)
		}
	}

	cmd = exec.Command(mkfs, "-d", tmpDir, path)
	cmd.Stderr = errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("while creating ext3 partition in %s: %s\nCommand error: %s", path, err, errBuf)
	}
	errBuf.Reset()
	return nil
}

// OverlayCreate creates an overlay at imgPath, or adds an overlay to imgPath if
// it is a SIF file. The overlay will have specified size (MiB) and sparseness.
// Any directories listed in overlayDirs will be created in the overlay fs.
func OverlayCreate(imgPath string, size int, sparse bool, overlayDirs ...string) error {
	if size < 64 {
		return fmt.Errorf("image size must be equal or greater than 64 MiB")
	}

	// If the imgPath exists, verify it's a SIF that we can add an overlay to.
	var img *image.Image
	if err := unix.Access(imgPath, unix.W_OK); err == nil {
		img, err = image.Init(imgPath, false)
		if err != nil {
			return fmt.Errorf("while opening image file %s: %s", imgPath, err)
		}
		_, err = canAddOverlay(img)
		if err != nil {
			return err
		}
	}

	// Create the overlay in a separate file.
	tmpFile := imgPath + ".ext3"
	defer os.Remove(tmpFile)
	if err := createOverlayFile(tmpFile, size, sparse, overlayDirs...); err != nil {
		return err
	}

	// No existing image - move overlay file to permanent location.
	if img == nil {
		if err := os.Rename(tmpFile, imgPath); err != nil {
			return fmt.Errorf("while renaming %s to %s: %s", tmpFile, imgPath, err)
		}
		return nil
	}
	// Add to OCI-SIF
	if img.Type == image.OCISIF {
		return ocisif.AddOverlay(imgPath, tmpFile)
	}
	// Add to Native SIF
	return addOverlayToSIF(imgPath, tmpFile)
}
