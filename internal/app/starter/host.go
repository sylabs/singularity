// Copyright (c) 2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package starter

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/sylabs/singularity/v4/internal/pkg/runtime/engine"
	"github.com/sylabs/singularity/v4/pkg/sylog"
)

func PostStartHost(postStartSocket int, e *engine.Engine) {
	sylog.Debugf("Entering PostStartHost")
	comm := os.NewFile(uintptr(postStartSocket), "unix")
	conn, err := net.FileConn(comm)
	if err != nil {
		sylog.Fatalf("socket communication error: %s\n", err)
	}
	comm.Close()
	defer conn.Close()

	ctx := context.TODO()

	// Wait for a write into the socket from master to trigger after container process started.
	data := make([]byte, 1)
	if _, err := conn.Read(data); err != nil {
		sylog.Fatalf("While reading from post start socket: %s", err)
	}

	if err := e.PostStartHost(ctx); err != nil {
		if _, err := conn.Write([]byte{'f'}); err != nil {
			sylog.Fatalf("Could not write to master: %s", err)
		}
		sylog.Fatalf("While running host post start tasks: %s", err)
	}

	if _, err := conn.Write([]byte{'c'}); err != nil {
		sylog.Fatalf("Could not write to master: %s", err)
	}
	sylog.Debugf("Exiting PostStartHost")
	os.Exit(0)
}

func CleanupHost(cleanupSocket int, e *engine.Engine) {
	sylog.Debugf("Entering CleanupHost")

	// An unclean shutdown, in which the parent of the cleanup process exits,
	// will result in SIGTERM (as this was set as the parent death signal in the
	// C starter code).
	tc := make(chan os.Signal, 1)
	signal.Notify(tc, syscall.SIGTERM)

	// A clean container shutdown results in the master process sending a
	// message on the cleanup socket to trigger cleanup. We will use SIGHUP to indicate this.
	comm := os.NewFile(uintptr(cleanupSocket), "unix")
	conn, err := net.FileConn(comm)
	if err != nil {
		sylog.Fatalf("socket communication error: %s\n", err)
	}
	comm.Close()
	defer conn.Close()

	go func() {
		data := make([]byte, 1)
		_, err := conn.Read(data)
		// Clean shutdown - master should be notified after cleanup.
		if err == nil {
			tc <- syscall.SIGUSR1
			return
		}
		// Unclean shutdown - master not available to notify after cleanup.
		tc <- syscall.SIGTERM
		sylog.Debugf("While reading from cleanup socket: %s", err)
	}()

	// Block here until direct SIGTERM, or generated SIGHUP from reading master
	// socket, is received.
	sig := <-tc
	sylog.Debugf("CleanupHost Signaled: %v", sig)

	// Run engine specific cleanup tasks
	err = e.CleanupHost(context.TODO())

	// If we are in a clean (master initiated) shutdown, notify master of any cleanup failure.
	if err != nil && sig == syscall.SIGUSR1 {
		if _, err := conn.Write([]byte{'f'}); err != nil {
			sylog.Debugf("Could not write to master: %s", err)
		}
	}
	// Exit on cleanup failure.
	if err != nil {
		sylog.Errorf("While running host cleanup tasks: %s", err)
		sylog.Debugf("Exiting CleanupHost - Cleanup failure")
		os.Exit(1)
	}

	// If we are in a clean (master initiated) shutdown, notify master of the cleanup success.
	if sig == syscall.SIGUSR1 {
		if _, err := conn.Write([]byte{'c'}); err != nil {
			sylog.Debugf("Could not write to master: %s", err)
			sylog.Debugf("Exiting CleanupHost - Master socket write failure")
			os.Exit(1)
		}
	}

	sylog.Debugf("Exiting CleanupHost - Success")
	os.Exit(0)
}
