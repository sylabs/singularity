// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package rpc

import (
	"encoding/gob"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// MountArgs defines the arguments to mount.
type MountArgs struct {
	Source     string
	Target     string
	Filesystem string
	Mountflags uintptr
	Data       string
}

// DecryptArgs defines the arguments to decrypt.
type DecryptArgs struct {
	Offset    uint64
	Loopdev   string
	Key       []byte
	MasterPid int
}

// MkdirArgs defines the arguments to mkdir.
type MkdirArgs struct {
	Path string
	Perm os.FileMode
}

// ChrootArgs defines the arguments to chroot.
type ChrootArgs struct {
	Root   string
	Method string
}

// LoopArgs defines the arguments to create a loop device.
type LoopArgs struct {
	Image      string
	Mode       int
	Info       unix.LoopInfo64
	MaxDevices int
	Shared     bool
}

// HostnameArgs defines the arguments to sethostname.
type HostnameArgs struct {
	Hostname string
}

// ChdirArgs defines the arguments to chdir.
type ChdirArgs struct {
	Dir string
}

// StatReply defines the reply for stat.
type StatReply struct {
	Fi  os.FileInfo
	Err error
}

// StatArgs defines the arguments to stat.
type StatArgs struct {
	Path string
}

// SendFuseFdArgs defines the arguments to send fuse file descriptor.
type SendFuseFdArgs struct {
	Socket int
	Fds    []int
}

// NvCCLIArgs defines the arguments to NvCCLI.
type NvCCLIArgs struct {
	Flags      []string
	RootFsPath string
	UserNS     bool
}

// FileInfo returns FileInfo interface to be passed as RPC argument.
func FileInfo(fi os.FileInfo) os.FileInfo {
	return &fileInfo{
		N:  fi.Name(),
		S:  fi.Size(),
		M:  fi.Mode(),
		T:  fi.ModTime(),
		Sy: fi.Sys(),
	}
}

// fileInfo internal interface with exported fields.
type fileInfo struct {
	N  string
	S  int64
	M  os.FileMode
	T  time.Time
	Sy any
}

func (fi fileInfo) Name() string {
	return fi.N
}

func (fi fileInfo) Size() int64 {
	return fi.S
}

func (fi fileInfo) Mode() os.FileMode {
	return fi.M
}

func (fi fileInfo) ModTime() time.Time {
	if fi.T.IsZero() {
		return time.Now()
	}
	return fi.T
}

func (fi fileInfo) IsDir() bool {
	return fi.M.IsDir()
}

func (fi fileInfo) Sys() any {
	return fi.Sy
}

func init() {
	gob.Register(syscall.Errno(0))
	gob.Register((*fileInfo)(nil))
	gob.Register((*syscall.Stat_t)(nil))
	gob.Register((*os.PathError)(nil))
	gob.Register((*os.SyscallError)(nil))
	gob.Register((*os.LinkError)(nil))
}
