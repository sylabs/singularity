// Copyright (c) 2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

//go:build !cgo

// Non-CGO fallback functions, for any projects that indirectly import this
// package as a dependency. Is not used from singularity itself. Does not
// populate the shell field. Sets Gecos field to os/user User.Name.

package user

import (
	"os/user"
	"strconv"
)

func convertUser(u *user.User) (*User, error) {
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	return &User{
		Name:  u.Username,
		UID:   uint32(uid),
		GID:   uint32(gid),
		Gecos: u.Name,
		Dir:   u.HomeDir,
	}, nil
}

func convertGroup(g *user.Group) (*Group, error) {
	gid, err := strconv.ParseUint(g.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	return &Group{
		Name: g.Name,
		GID:  uint32(gid),
	}, nil
}

func current() (*User, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return convertUser(u)
}

func lookupUser(username string) (*User, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return nil, err
	}
	return convertUser(u)
}

func lookupUserId(uid string) (*User, error) {
	u, err := user.LookupId(uid)
	if err != nil {
		return nil, err
	}
	return convertUser(u)
}

func lookupUnixUid(uid int) (*User, error) {
	return lookupUserId(strconv.Itoa(uid))
}

func currentGroup() (*Group, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return lookupGroupId(u.Gid)
}

func lookupGroup(groupname string) (*Group, error) {
	g, err := user.LookupGroup(groupname)
	if err != nil {
		return nil, err
	}
	return convertGroup(g)
}

func lookupGroupId(gid string) (*Group, error) {
	g, err := user.LookupGroupId(gid)
	if err != nil {
		return nil, err
	}
	return convertGroup(g)
}

func lookupUnixGid(gid int) (*Group, error) {
	return lookupGroupId(strconv.Itoa(gid))
}
