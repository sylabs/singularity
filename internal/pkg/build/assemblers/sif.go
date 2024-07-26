// Copyright (c) 2018-2024, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package assemblers

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"syscall"

	"github.com/sylabs/sif/v2/pkg/sif"
	"github.com/sylabs/singularity/v4/internal/pkg/util/crypt"
	"github.com/sylabs/singularity/v4/internal/pkg/util/fs/squashfs"
	"github.com/sylabs/singularity/v4/internal/pkg/util/machine"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/cryptkey"
)

// SIFAssembler doesn't store anything.
type SIFAssembler struct{}

type encryptionOptions struct {
	keyInfo   cryptkey.KeyInfo
	plaintext []byte
}

func createSIF(path string, b *types.Bundle, squashfile string, encOpts *encryptionOptions, arch string) (err error) {
	var dis []sif.DescriptorInput

	// data we need to create a definition file descriptor
	definput, err := sif.NewDescriptorInput(sif.DataDeffile, bytes.NewReader(b.Recipe.FullRaw))
	if err != nil {
		return err
	}

	// add this descriptor input element to creation descriptor slice
	dis = append(dis, definput)

	// add all JSON data object within SIF by alphabetical order
	sorted := make([]string, 0, len(b.JSONObjects))
	for name := range b.JSONObjects {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		if len(b.JSONObjects[name]) > 0 {
			// data we need to create a definition file descriptor
			in, err := sif.NewDescriptorInput(sif.DataGenericJSON, bytes.NewReader(b.JSONObjects[name]),
				sif.OptObjectName(name),
			)
			if err != nil {
				return err
			}

			// add this descriptor input element to creation descriptor slice
			dis = append(dis, in)
		}
	}

	// open up the data object file for this descriptor
	fp, err := os.Open(squashfile)
	if err != nil {
		return fmt.Errorf("while opening partition file: %s", err)
	}
	defer fp.Close()

	fs := sif.FsSquash
	if encOpts != nil {
		fs = sif.FsEncryptedSquashfs
	}

	// data we need to create a system partition descriptor
	parinput, err := sif.NewDescriptorInput(sif.DataPartition, fp,
		sif.OptPartitionMetadata(fs, sif.PartPrimSys, arch),
	)
	if err != nil {
		return err
	}

	// add this descriptor input element to the list
	dis = append(dis, parinput)

	if encOpts != nil {
		data, err := cryptkey.EncryptKey(encOpts.keyInfo, encOpts.plaintext)
		if err != nil {
			return fmt.Errorf("while encrypting filesystem key: %s", err)
		}

		if data != nil {
			syspartID := uint32(len(dis))
			part, err := sif.NewDescriptorInput(sif.DataCryptoMessage, bytes.NewReader(data),
				sif.OptLinkedID(syspartID),
				sif.OptCryptoMessageMetadata(sif.FormatPEM, sif.MessageRSAOAEP),
			)
			if err != nil {
				return err
			}

			dis = append(dis, part)
		}
	}

	// remove anything that may exist at the build destination at last moment
	os.RemoveAll(path)

	f, err := sif.CreateContainerAtPath(path,
		sif.OptCreateWithLaunchScript("#!/usr/bin/env run-singularity\n"),
		sif.OptCreateWithDescriptors(dis...),
	)
	if err != nil {
		return fmt.Errorf("while creating container: %w", err)
	}

	if err := f.UnloadContainer(); err != nil {
		return fmt.Errorf("while unloading container: %w", err)
	}

	// chown the sif file to the calling user
	if uid, gid, ok := changeOwner(); ok {
		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("while changing image ownership: %s", err)
		}
	}

	return nil
}

// Assemble creates a SIF image from a Bundle.
func (a *SIFAssembler) Assemble(b *types.Bundle, path string) error {
	sylog.Infof("Creating SIF file...")

	f, err := os.CreateTemp(b.TmpDir, "squashfs-")
	if err != nil {
		return fmt.Errorf("while creating temporary file for squashfs: %v", err)
	}

	fsPath := f.Name()
	f.Close()
	defer os.Remove(fsPath)

	arch := machine.ArchFromContainer(b.RootfsPath)
	if arch == "" {
		sylog.Infof("Architecture not recognized, use native")
		arch = runtime.GOARCH
	}
	sylog.Verbosef("Set SIF container architecture to %s", arch)

	// Squash ownership of squashfs files to root when called as non-root, so we
	// don't have container files owned by a uid that might not exist on other
	// systems.
	allroot := syscall.Getuid() != 0

	if err := squashfs.Mksquashfs([]string{b.RootfsPath}, fsPath, squashfs.OptAllRoot(allroot)); err != nil {
		return fmt.Errorf("while creating squashfs: %v", err)
	}

	var encOpts *encryptionOptions

	if b.Opts.EncryptionKeyInfo != nil {
		plaintext, err := cryptkey.NewPlaintextKey(*b.Opts.EncryptionKeyInfo)
		if err != nil {
			return fmt.Errorf("unable to obtain encryption key: %+v", err)
		}

		// A dm-crypt device needs to be created with squashfs
		cryptDev := &crypt.Device{}

		// TODO (schebro): Fix #3876
		// Detach the following code from the squashfs creation. SIF can be
		// created first and encrypted after. This gives the flexibility to
		// encrypt an existing SIF
		loopPath, err := cryptDev.EncryptFilesystem(fsPath, plaintext)
		if err != nil {
			return fmt.Errorf("unable to encrypt filesystem at %s: %+v", fsPath, err)
		}
		defer os.Remove(loopPath)

		fsPath = loopPath

		encOpts = &encryptionOptions{
			keyInfo:   *b.Opts.EncryptionKeyInfo,
			plaintext: plaintext,
		}
	}

	err = createSIF(path, b, fsPath, encOpts, arch)
	if err != nil {
		return fmt.Errorf("while creating SIF: %v", err)
	}

	return nil
}

// changeOwner check the command being called with sudo with the environment
// variable SUDO_COMMAND. Pattern match that for the singularity bin.
func changeOwner() (int, int, bool) {
	r := regexp.MustCompile("(singularity)")
	sudoCmd := os.Getenv("SUDO_COMMAND")
	if !r.MatchString(sudoCmd) {
		return 0, 0, false
	}

	if os.Getenv("SUDO_USER") == "" || syscall.Getuid() != 0 {
		return 0, 0, false
	}

	_uid := os.Getenv("SUDO_UID")
	_gid := os.Getenv("SUDO_GID")
	if _uid == "" || _gid == "" {
		sylog.Warningf("Env vars SUDO_UID or SUDO_GID are not set, won't call chown over built SIF")

		return 0, 0, false
	}

	uid, err := strconv.Atoi(_uid)
	if err != nil {
		sylog.Warningf("Error while calling strconv: %v", err)

		return 0, 0, false
	}
	gid, err := strconv.Atoi(_gid)
	if err != nil {
		sylog.Warningf("Error while calling strconv : %v", err)

		return 0, 0, false
	}

	return uid, gid, true
}
