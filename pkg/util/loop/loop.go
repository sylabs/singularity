// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2021, Genomics plc.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package loop

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/fs/lock"
	"golang.org/x/sys/unix"
)

// Device describes a loop device
type Device struct {
	MaxLoopDevices int
	Shared         bool
	Info           *unix.LoopInfo64
	fd             *int
}

// errTransientAttach is used to indicate hitting errors within loop device setup that are transient.
// These may be cleared by our automatic retries, or by the user re-running.
var errTransientAttach = errors.New("transient error, please retry")

// Error retry attempts & interval
const (
	maxRetries    = 5
	retryInterval = 250 * time.Millisecond
)

// AttachFromFile attempts to find a suitable loop device to use for the specified image.
// It runs through /dev/loopXX, up to MaxLoopDevices to find a free loop device, or
// to share a loop device already associated to file (if shared loop devices are enabled).
// If a usable loop device is found, then loop.Fd is set and no error is returned.
// If a usable loop device is not found, and this is due to a transient EAGAIN / EBUSY error,
// then it will retry up to maxRetries times, retryInterval apart, before returning an error.
func (loop *Device) AttachFromFile(image *os.File, mode int, number *int) error {
	var err error

	if image == nil {
		return fmt.Errorf("empty file pointer")
	}
	fi, err := image.Stat()
	if err != nil {
		return err
	}

	//nolint:forcetypeassert
	st := fi.Sys().(*syscall.Stat_t)
	imageIno := st.Ino
	// cast to uint64 as st.Dev is uint32 on MIPS
	imageDev := uint64(st.Dev)

	if loop.Shared {
		ok, err := loop.shareLoop(imageIno, imageDev, mode, number)
		if err != nil {
			return err
		}
		// We found a shared loop device, and loop.Fd was set
		if ok {
			return nil
		}
		loop.Shared = false
	}

	for i := 0; i < maxRetries; i++ {
		err = loop.attachLoop(image, mode, number)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errTransientAttach) {
			return err
		}
		// At least one error while we were working through loop devices was a transient one
		// that should resolve by itself, so let's try again!
		sylog.Debugf("%v", err)
		time.Sleep(retryInterval)
	}
	return fmt.Errorf("failed to attach loop device: %s", err)
}

// shareLoop runs over /dev/loopXX devices, looking for one that already has our image attached.
// If a loop device can be shared, loop.Fd is set, and ok will be true.
// If no loop device can be shared, ok will be false.
func (loop *Device) shareLoop(imageIno, imageDev uint64, mode int, number *int) (ok bool, err error) {
	// Because we hold a lock on /dev here, avoid delayed retries inside this function,
	// as it could impact parallel startup of many instances of Singularity or
	// other programs.
	fd, err := lock.Exclusive("/dev")
	if err != nil {
		return false, err
	}
	defer lock.Release(fd)

	for device := 0; device < loop.MaxLoopDevices; device++ {
		*number = device

		// Try to open an existing loop device, but don't create a new one
		loopFd, err := openLoopDev(device, mode, false)
		if err != nil {
			if !os.IsNotExist(err) {
				sylog.Debugf("Couldn't open loop device %d: %v\n", device, err)
			}
			continue
		}

		status, err := GetStatusFromFd(uintptr(loopFd))
		if err != nil {
			syscall.Close(loopFd)
			sylog.Debugf("Couldn't get status from loop device %d: %v\n", device, err)
			continue
		}

		if status.Inode == imageIno && status.Device == imageDev &&
			status.Flags&unix.LO_FLAGS_READ_ONLY == loop.Info.Flags&unix.LO_FLAGS_READ_ONLY &&
			status.Offset == loop.Info.Offset && status.Sizelimit == loop.Info.Sizelimit {
			// keep the reference to the loop device file descriptor to
			// be sure that the loop device won't be released between this
			// check and the mount of the filesystem
			sylog.Debugf("Sharing loop device %d", device)
			loop.fd = new(int)
			*loop.fd = loopFd
			return true, nil
		}
		syscall.Close(loopFd)
	}
	return false, nil
}

// attachLoop will find a free /dev/loopXX device, or create a new one, and attach image to it.
// For most failures with loopN, it will try loopN+1, continuing up to loop.MaxLoopDevices.
// If there was an EAGAIN/EBUSY error on setting loop flags this is transient, and the returned
// errTransientAttach indicates it is likely worth trying again.
func (loop *Device) attachLoop(image *os.File, mode int, number *int) error {
	var path string
	// Keep track of the last transient error we hit (if any)
	// If we fail to find a loop device, but hit at least one transient error then it's worth trying again.
	var transientError error

	// Because we hold a lock on /dev here, avoid delayed retries inside this function,
	// as it could impact parallel startup of many instances of Singularity or
	// other programs.
	fd, err := lock.Exclusive("/dev")
	if err != nil {
		return err
	}
	defer lock.Release(fd)

	for device := 0; device < loop.MaxLoopDevices; device++ {
		*number = device

		// Try to open the loop device, creating the device node if needed
		loopFd, err := openLoopDev(device, mode, true)
		if err != nil {
			sylog.Debugf("couldn't open loop device %d: %v", device, err)
			continue
		}

		if err := unix.IoctlSetInt(loopFd, unix.LOOP_SET_FD, int(image.Fd())); err != nil {
			// On error, we'll move on to try the next loop device
			syscall.Close(loopFd)
			continue
		}

		if _, _, err := syscall.Syscall(syscall.SYS_FCNTL, uintptr(loopFd), syscall.F_SETFD, syscall.FD_CLOEXEC); err != 0 {
			syscall.Close(loopFd)
			return fmt.Errorf("failed to set close-on-exec on loop device %s: %s", path, err.Error())
		}

		if err := unix.IoctlLoopSetStatus64(loopFd, loop.Info); err != nil {
			// If we hit an error then dissociate our image from the loop device
			unix.IoctlSetInt(loopFd, unix.LOOP_CLR_FD, 0)
			// EAGAIN and EBUSY will likely clear themselves... so track we hit one and keep trying
			if err == unix.EAGAIN || err == unix.EBUSY {
				syscall.Close(loopFd)
				sylog.Debugf("transient error %v for loop device %d, continuing", err, device)
				transientError = err
				continue
			}
			return fmt.Errorf("failed to set loop flags on loop device: %s", err)
		}

		loop.fd = new(int)
		*loop.fd = loopFd
		return nil
	}

	if transientError != nil {
		return fmt.Errorf("%w: %v", errTransientAttach, transientError)
	}

	return fmt.Errorf("no loop devices available")
}

// openLoopDev will attempt to open the specified loop device number, with
// specified mode. If it is not present in /dev, and create is true,
// /dev/loop-control will be used to create it. Returns the fd for the opened
// device, or -1 if it was not possible to open it.
func openLoopDev(device, mode int, create bool) (loopFd int, err error) {
	path := fmt.Sprintf("/dev/loop%d", device)
	fi, err := os.Stat(path)

	if os.IsNotExist(err) {
		if !create {
			return -1, err
		}

		err := addLoopDev(device)
		if err != nil && err != unix.EEXIST {
			return -1, err
		}
	} else {
		// If there's another stat error that's likely fatal.. we're done..
		if err != nil {
			return -1, fmt.Errorf("could not stat %s: %w", path, err)
		}

		if fi.Mode()&os.ModeDevice == 0 {
			return -1, fmt.Errorf("%s is not a block device", path)
		}
	}

	// Now open the loop device
	loopFd, err = syscall.Open(path, mode, 0o600)
	if err != nil {
		return -1, fmt.Errorf("could not open %s: %w", path, err)
	}
	return loopFd, nil
}

// addLoopDev will create a loop device via /dev/loop-control.
func addLoopDev(device int) error {
	const loopControl = "/dev/loop-control"

	lc, err := os.OpenFile(loopControl, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("while opening loop-control device: %w", err)
	}
	defer lc.Close()

	sylog.Debugf("LOOP_CTL_ADD for loop device %d", device)
	if err := unix.IoctlSetInt(int(lc.Fd()), unix.LOOP_CTL_ADD, device); err != nil {
		return err
	}

	// Verify that the loop device was created where we can see it.
	// If it's not there, try to create it with mknod. This might be necessary where
	// we are running nested in a container, and the new /dev/loopXX didn't propagate
	// through the /dev mount.
	path := fmt.Sprintf("/dev/loop%d", device)
	_, err = os.Stat(path)

	if os.IsNotExist(err) {
		sylog.Debugf("Expected loop device %d is not visible. Creating with mknod", device)
		dev := int((7 << 8) | (device & 0xff) | ((device & 0xfff00) << 12))
		if err := syscall.Mknod(path, syscall.S_IFBLK|0o660, dev); err != nil {
			return err
		}
	}

	return err
}

// AttachFromPath finds a free loop device, opens it, and stores file descriptor
// of opened image path
func (loop *Device) AttachFromPath(image string, mode int, number *int) error {
	file, err := os.OpenFile(image, mode, 0o600)
	if err != nil {
		return err
	}
	return loop.AttachFromFile(file, mode, number)
}

// Close closes the loop device.
func (loop *Device) Close() error {
	if loop.fd != nil {
		return syscall.Close(*loop.fd)
	}
	return nil
}

// GetStatusFromFd gets info status about an opened loop device
func GetStatusFromFd(fd uintptr) (*unix.LoopInfo64, error) {
	info, err := unix.IoctlLoopGetStatus64(int(fd))
	if err != nil {
		return nil, fmt.Errorf("failed to get loop flags for loop device: %s", err)
	}
	return info, nil
}

// GetStatusFromPath gets info status about a loop device from path
func GetStatusFromPath(path string) (*unix.LoopInfo64, error) {
	loop, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open loop device %s: %s", path, err)
	}
	return GetStatusFromFd(loop.Fd())
}
