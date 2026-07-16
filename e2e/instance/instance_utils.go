// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package instance

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/sylabs/singularity/v4/e2e/internal/e2e"
)

type instance struct {
	Image    string `json:"img"`
	Instance string `json:"instance"`
	Pid      int    `json:"pid"`
}

type instanceList struct {
	Instances []instance `json:"instances"`
}

func getFreePorts(t *testing.T, n int) []int {
	t.Helper()

	listeners := make([]net.Listener, 0, n)
	ports := make([]int, 0, n)
	for range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to allocate a tcp port: %v", err)
		}
		listeners = append(listeners, l)
		//nolint:forcetypeassert
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	for _, l := range listeners {
		l.Close()
	}
	return ports
}

func getFreePort(t *testing.T) int {
	t.Helper()
	return getFreePorts(t, 1)[0]
}

//nolint:unparam
func (c *ctx) stopInstance(t *testing.T, instance string, stopArgs ...string) (stdout string, stderr string, success bool) {
	args := stopArgs

	if instance != "" {
		args = append(args, instance)
	}

	c.env.RunSingularity(
		t,
		e2e.WithProfile(c.profile),
		e2e.WithCommand("instance stop"),
		e2e.WithArgs(args...),
		e2e.PostRun(func(t *testing.T) {
			success = !t.Failed()
		}),
		e2e.ExpectExit(0, e2e.GetStreams(&stdout, &stderr)),
	)

	c.expectInstance(t, instance, 0)

	return
}

//nolint:unparam
func (c *ctx) execInstance(t *testing.T, instance string, execArgs ...string) (stdout string, stderr string, success bool) {
	args := make([]string, 0, 1+len(execArgs))
	args = append(args, "instance://"+instance)
	args = append(args, execArgs...)

	c.env.RunSingularity(
		t,
		e2e.WithProfile(c.profile),
		e2e.WithCommand("exec"),
		e2e.WithArgs(args...),
		e2e.PostRun(func(t *testing.T) {
			success = !t.Failed()
		}),
		e2e.ExpectExit(0, e2e.GetStreams(&stdout, &stderr)),
	)

	return
}

// Check if there is the number of expected instances with the provided name.
//
// Poll to a timeout to accommodate delays in instance startup / shutdown on
// busy systems.
func (c *ctx) expectInstance(t *testing.T, name string, nb int) {
	t.Helper()

	const (
		timeout  = 10 * time.Second
		interval = 100 * time.Millisecond
	)

	deadline := time.Now().Add(timeout)
	count := -1
	for {
		var stdout, stderr string
		c.env.RunSingularity(
			t,
			e2e.WithProfile(c.profile),
			e2e.WithCommand("instance list"),
			e2e.WithArgs("--json", name),
			e2e.ExpectExit(0, e2e.GetStreams(&stdout, &stderr)),
		)
		if t.Failed() {
			return
		}

		var instances instanceList
		if err := json.Unmarshal([]byte(stdout), &instances); err != nil {
			t.Errorf("Error while decoding JSON from 'instance list': %v", err)
			return
		}

		count = len(instances.Instances)
		if count == nb || time.Now().After(deadline) {
			break
		}
		time.Sleep(interval)
	}

	if count != nb {
		t.Errorf("%d instance %q found, expected %d", count, name, nb)
	}
}

// Sends a deterministic message to an echo server and expects the same, or a
// reversed, message in response.
func echo(t *testing.T, port int, reverse bool) {
	t.Helper()

	const (
		message         = "b40cbeaaea293f7e8bd40fb61f389cfca9823467\n"
		reversedMessage = "7643289acfc983f16bf04db8e7f392aeaaebc04b\n"
		retries         = 5
	)

	expectResponse := message
	if reverse {
		expectResponse = reversedMessage
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	var lastErr error
	for attempt := range retries {
		if attempt > 0 {
			time.Sleep(time.Second)
		}

		sock, err := net.Dial("tcp", addr)
		if err != nil {
			lastErr = fmt.Errorf("dial: %w", err)
			continue
		}

		if _, err := fmt.Fprint(sock, message); err != nil {
			sock.Close()
			lastErr = fmt.Errorf("write: %w", err)
			continue
		}

		response, err := bufio.NewReader(sock).ReadString('\n')
		sock.Close()
		if err != nil {
			lastErr = fmt.Errorf("read: %w", err)
			continue
		}
		if response != expectResponse {
			lastErr = fmt.Errorf("response %q != %q", response, expectResponse)
			continue
		}

		return
	}

	t.Errorf("echo server on port %d did not respond correctly after %d attempts: %v", port, retries, lastErr)
}
