/*
 * Copyright (c) 2017-2019, SyLabs, Inc. All rights reserved.
 * Copyright (c) 2017, SingularityWare, LLC. All rights reserved.
 *
 * This software is licensed under a 3-clause BSD license.  Please
 * consult LICENSE.md file distributed with the sources of this project regarding
 * your rights to use or distribute this software.
 *
 */

 /* When modifying this file, you must run `go clean -cache` before building for
 * changes to be picked up. 
 * See: https://pkg.go.dev/cmd/go#hdr-Build_and_test_caching
 */

#define _GNU_SOURCE
#include <unistd.h>
#include <sys/syscall.h>

#include "include/capability.h"

int capget(cap_user_header_t hdrp, cap_user_data_t datap) {
    return syscall(__NR_capget, hdrp, datap);
}

int capset(cap_user_header_t hdrp, const cap_user_data_t datap) {
    return syscall(__NR_capset, hdrp, datap);
}
