// Copyright (c) 2019-2025, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package namespaces

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ccoveille/go-safecast"
)

// IsInsideUserNamespace checks if a process is already running in a
// user namespace and also returns if the process has permissions to use
// setgroups in this user namespace.
func IsInsideUserNamespace(pid int) (bool, bool) {
	// default values returned in case of error
	insideUserNs := false
	setgroupsAllowed := false

	// can fail if the kernel doesn't support user namespace
	r, err := os.Open(fmt.Sprintf("/proc/%d/uid_map", pid))
	if err != nil {
		return insideUserNs, setgroupsAllowed
	}
	defer r.Close()

	scanner := bufio.NewScanner(r)
	// we are interested only by the first line of
	// uid_map which would give us the answer quickly
	// based on the value of size field
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())

		// trust values returned by procfs
		size, _ := strconv.ParseUint(fields[2], 10, 32)

		// a size of 4294967295 means the process is running
		// in the host user namespace
		if uint32(size) == ^uint32(0) {
			return insideUserNs, setgroupsAllowed
		}

		// process is running inside user namespace
		insideUserNs = true

		// should not fail if open call passed
		d, err := os.ReadFile(fmt.Sprintf("/proc/%d/setgroups", pid))
		if err != nil {
			return insideUserNs, setgroupsAllowed
		}
		setgroupsAllowed = string(d) == "allow\n"
	}

	return insideUserNs, setgroupsAllowed
}

// HostUID attempts to find the original host UID if the current
// process is running inside a user namespace, if it doesn't it
// simply returns the current UID
func HostUID() (uint32, error) {
	const uidMap = "/proc/self/uid_map"

	currentUID, err := safecast.ToUint32(os.Getuid())
	if err != nil {
		return 0, err
	}

	f, err := os.Open(uidMap)
	if err != nil {
		if !os.IsNotExist(err) {
			return 0, fmt.Errorf("failed to read: %s: %s", uidMap, err)
		}
		// user namespace not supported
		return currentUID, nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())

		size, err := strconv.ParseUint(fields[2], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("failed to convert size field %s: %s", fields[2], err)
		}
		// not in a user namespace, use current UID
		if uint32(size) == ^uint32(0) {
			break
		}

		// we are inside a user namespace
		parsedID, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("failed to convert container UID field %s: %s", fields[0], err)
		}
		containerUID := uint32(parsedID)
		// we can safely assume that a user won't have two
		// consequent UID and we look if current UID match
		// a 1:1 user mapping
		if size == 1 && currentUID == containerUID {
			uid, err := strconv.ParseUint(fields[1], 10, 32)
			if err != nil {
				return 0, fmt.Errorf("failed to convert host UID field %s: %s", fields[1], err)
			}
			return uint32(uid), nil
		}
	}

	// return current UID by default
	return currentUID, nil
}
