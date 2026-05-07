// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package client

import (
	"net/rpc"
	"os"

	args "github.com/sylabs/singularity/v4/internal/pkg/runtime/engine/singularity/rpc"
	"golang.org/x/sys/unix"
)

// RPC holds the state necessary for remote procedure calls.
type RPC struct {
	Client *rpc.Client
	Name   string
}

// Mount calls the mount RPC using the supplied arguments.
func (t *RPC) Mount(source string, target string, filesystem string, flags uintptr, data string) error {
	arguments := &args.MountArgs{
		Source:     source,
		Target:     target,
		Filesystem: filesystem,
		Mountflags: flags,
		Data:       data,
	}

	var mountErr error

	err := t.Client.Call(t.Name+".Mount", arguments, &mountErr)
	// RPC communication will take precedence over mount error
	if err == nil {
		err = mountErr
	}

	return err
}

// Decrypt calls the DeCrypt RPC using the supplied arguments.
func (t *RPC) Decrypt(offset uint64, path string, key []byte, masterPid int) (string, error) {
	arguments := &args.DecryptArgs{
		Offset:    offset,
		Loopdev:   path,
		Key:       key,
		MasterPid: masterPid,
	}

	var reply string
	err := t.Client.Call(t.Name+".Decrypt", arguments, &reply)

	return reply, err
}

// Mkdir calls the mkdir RPC using the supplied arguments.
func (t *RPC) Mkdir(path string, perm os.FileMode) error {
	arguments := &args.MkdirArgs{
		Path: path,
		Perm: perm,
	}
	return t.Client.Call(t.Name+".Mkdir", arguments, nil)
}

// Chroot calls the chroot RPC using the supplied arguments.
func (t *RPC) Chroot(root string, method string) (int, error) {
	arguments := &args.ChrootArgs{
		Root:   root,
		Method: method,
	}
	var reply int
	err := t.Client.Call(t.Name+".Chroot", arguments, &reply)
	return reply, err
}

// LoopDevice calls the loop device RPC using the supplied arguments.
func (t *RPC) LoopDevice(image string, mode int, info unix.LoopInfo64, maxDevices int, shared bool) (int, error) {
	arguments := &args.LoopArgs{
		Image:      image,
		Mode:       mode,
		Info:       info,
		MaxDevices: maxDevices,
		Shared:     shared,
	}
	var reply int
	err := t.Client.Call(t.Name+".LoopDevice", arguments, &reply)
	return reply, err
}

// SetHostname calls the sethostname RPC using the supplied arguments.
func (t *RPC) SetHostname(hostname string) (int, error) {
	arguments := &args.HostnameArgs{
		Hostname: hostname,
	}
	var reply int
	err := t.Client.Call(t.Name+".SetHostname", arguments, &reply)
	return reply, err
}

// Chdir calls the chdir RPC using the supplied arguments.
func (t *RPC) Chdir(dir string) (int, error) {
	arguments := &args.ChdirArgs{
		Dir: dir,
	}
	var reply int
	err := t.Client.Call(t.Name+".Chdir", arguments, &reply)
	return reply, err
}

// Stat calls the stat RPC using the supplied arguments.
func (t *RPC) Stat(path string) (os.FileInfo, error) {
	arguments := &args.StatArgs{
		Path: path,
	}
	var reply args.StatReply
	err := t.Client.Call(t.Name+".Stat", arguments, &reply)
	if err != nil {
		return nil, err
	}
	return reply.Fi, reply.Err
}

// Lstat calls the lstat RPC using the supplied arguments.
func (t *RPC) Lstat(path string) (os.FileInfo, error) {
	arguments := &args.StatArgs{
		Path: path,
	}
	var reply args.StatReply
	err := t.Client.Call(t.Name+".Lstat", arguments, &reply)
	if err != nil {
		return nil, err
	}
	return reply.Fi, reply.Err
}

// SendFuseFd calls the SendFuseFd RPC using the supplied arguments.
func (t *RPC) SendFuseFd(socket int, fds []int) error {
	arguments := &args.SendFuseFdArgs{
		Socket: socket,
		Fds:    fds,
	}
	var reply int
	err := t.Client.Call(t.Name+".SendFuseFd", arguments, &reply)
	return err
}

// NvCCLI will call nvidia-container-cli to configure GPU(s) for the container.
func (t *RPC) NvCCLI(flags []string, rootFsPath string, userNS bool) error {
	arguments := &args.NvCCLIArgs{
		Flags:      flags,
		RootFsPath: rootFsPath,
		UserNS:     userNS,
	}
	return t.Client.Call(t.Name+".NvCCLI", arguments, nil)
}
