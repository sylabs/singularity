// Copyright (c) 2018-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"fmt"
	"os"

	"github.com/sylabs/singularity/v4/cmd/internal/cli"
	"golang.org/x/sys/unix"
)

func main() {
	fh, err := os.OpenFile(os.Args[1], os.O_RDWR|os.O_CREATE|os.O_TRUNC|unix.O_NOFOLLOW, 0o644)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer fh.Close()

	if err := cli.GenBashCompletion(fh); err != nil {
		fmt.Println(err)
		return
	}
}
