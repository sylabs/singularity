// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

// Includes code from https://github.com/containers/podman
// Released under the Apache License Version 2.0

package oci

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/moby/term"
	"github.com/pkg/errors"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"golang.org/x/sys/unix"
)

var ErrDetach = errors.New("detached from container")

// attachStreams contains streams that will be attached to the container
type attachStreams struct {
	// OutputStream will be attached to container's STDOUT
	OutputStream io.Writer
	// ErrorStream will be attached to container's STDERR
	ErrorStream io.Writer
	// InputStream will be attached to container's STDIN
	InputStream io.Reader
	// AttachOutput is whether to attach to STDOUT
	// If false, stdout will not be attached
	AttachOutput bool
	// AttachError is whether to attach to STDERR
	// If false, stdout will not be attached
	AttachError bool
	// AttachInput is whether to attach to STDIN
	// If false, stdout will not be attached
	AttachInput bool
}

/* Sync with stdpipe_t in conmon.c */
const (
	AttachPipeStdin  = 1
	AttachPipeStdout = 2
	AttachPipeStderr = 3
)

// DetachKeys is the key sequence for detaching a container.
const DetachKeys = "ctrl-p,ctrl-q"

// Attach attaches the console to a running container
func Attach(containerID string) error {
	streams := attachStreams{
		OutputStream: os.Stdout,
		ErrorStream:  os.Stderr,
		InputStream:  bufio.NewReader(os.Stdin),
		AttachOutput: true,
		AttachError:  true,
		AttachInput:  true,
	}

	sd, err := stateDir(containerID)
	if err != nil {
		return fmt.Errorf("while computing state directory: %w", err)
	}
	attachSock := filepath.Join(sd, bundleLink, attachSocket)
	conn, err := openUnixSocket(attachSock)
	if err != nil {
		return fmt.Errorf("while connecting to attach socket: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			sylog.Errorf("while closing attach socket: %v", err)
		}
	}()

	detachKeys, err := processDetachKeys(DetachKeys)
	if err != nil {
		return fmt.Errorf("invalid detach key sequence: %w", err)
	}

	receiveStdoutError, stdinDone := setupStdioChannels(streams, conn, detachKeys)

	return readStdio(conn, streams, receiveStdoutError, stdinDone)
}

// The following utility functions are taken from https://github.com/containers/podman
// Released under the Apache License Version 2.0

func openUnixSocket(path string) (*net.UnixConn, error) {
	fd, err := unix.Open(path, unix.O_PATH, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	return net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: fmt.Sprintf("/proc/self/fd/%d", fd), Net: "unixpacket"})
}

func setupStdioChannels(streams attachStreams, conn *net.UnixConn, detachKeys []byte) (chan error, chan error) {
	receiveStdoutError := make(chan error)
	go func() {
		receiveStdoutError <- redirectResponseToOutputStreams(streams.OutputStream, streams.ErrorStream, streams.AttachOutput, streams.AttachError, conn)
	}()

	stdinDone := make(chan error)
	go func() {
		var err error
		if streams.AttachInput {
			_, err = copyDetachable(conn, streams.InputStream, detachKeys)
		}
		stdinDone <- err
	}()

	return receiveStdoutError, stdinDone
}

func redirectResponseToOutputStreams(outputStream, errorStream io.Writer, writeOutput, writeError bool, conn io.Reader) error {
	var err error
	buf := make([]byte, 8192+1) /* Sync with conmon STDIO_BUF_SIZE */
	for {
		nr, er := conn.Read(buf)
		if nr > 0 {
			var dst io.Writer
			var doWrite bool
			switch buf[0] {
			case AttachPipeStdout:
				dst = outputStream
				doWrite = writeOutput
			case AttachPipeStderr:
				dst = errorStream
				doWrite = writeError
			default:
				sylog.Infof("Received unexpected attach type %+d", buf[0])
			}
			if dst == nil {
				return errors.New("output destination cannot be nil")
			}

			if doWrite {
				nw, ew := dst.Write(buf[1:nr])
				if ew != nil {
					err = ew
					break
				}
				if nr != nw+1 {
					err = io.ErrShortWrite
					break
				}
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return err
}

func readStdio(conn *net.UnixConn, streams attachStreams, receiveStdoutError, stdinDone chan error) error {
	var err error
	select {
	case err = <-receiveStdoutError:
		conn.CloseWrite()
		return err
	case err = <-stdinDone:
		if err == ErrDetach {
			conn.CloseWrite()
			return err
		}
		if err == nil {
			// copy stdin is done, close it
			if connErr := conn.CloseWrite(); connErr != nil {
				sylog.Errorf("Unable to close conn: %v", connErr)
			}
		}
		if streams.AttachOutput || streams.AttachError {
			return <-receiveStdoutError
		}
	}
	return nil
}

func copyDetachable(dst io.Writer, src io.Reader, keys []byte) (written int64, err error) {
	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			preservBuf := []byte{}
			for i, key := range keys {
				preservBuf = append(preservBuf, buf[0:nr]...)
				if nr != 1 || buf[0] != key {
					break
				}
				if i == len(keys)-1 {
					return 0, ErrDetach
				}
				nr, er = src.Read(buf)
			}
			var nw int
			var ew error
			if len(preservBuf) > 0 {
				nw, ew = dst.Write(preservBuf)
				nr = len(preservBuf)
			} else {
				nw, ew = dst.Write(buf[0:nr])
			}
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func processDetachKeys(keys string) ([]byte, error) {
	// Check the validity of the provided keys first
	if len(keys) == 0 {
		return []byte{}, nil
	}
	detachKeys, err := term.ToBytes(keys)
	if err != nil {
		return nil, fmt.Errorf("invalid detach keys: %w", err)
	}
	return detachKeys, nil
}
