// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// Copyright (c) 2020, Control Command Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package docs

// Global content for help and man pages
const (

	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	// keyserver command
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	KeyserverUse   string = `keyserver [subcommand options...]`
	KeyserverShort string = `Manage singularity keyservers`
	KeyserverLong  string = `
  The 'keyserver' command allows you to manage standalone keyservers that will 
  be used for retrieving cryptographic keys.`
	KeyserverExample string = `
  All group commands have their own help output:

    $ singularity help keyserver add
    $ singularity keyserver add`
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	// keyserver add command
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	KeyserverAddUse   string = `add [options] [remoteName] <keyserver_url>`
	KeyserverAddShort string = `Add a keyserver (root user only)`
	KeyserverAddLong  string = `
  The 'keyserver add' command allows to define additional keyserver. The --order
  option can define the order of the keyserver for all related key operations, 
  therefore when specifying '--order 1' the keyserver will become the primary 
  keyserver. If no endpoint is specified, it will use the default remote
  endpoint (SylabsCloud).`
	KeyserverAddExample string = `
  $ singularity keyserver add https://keys.example.com

  To add a keyserver to be used as the primary keyserver for the current
  endpoint:
  $ singularity keyserver add --order 1 https://keys.example.com`
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	// keyserver remove command
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	KeyserverRemoveUse   string = `remove [remoteName] <keyserver_url>`
	KeyserverRemoveShort string = `Remove a keyserver (root user only)`
	KeyserverRemoveLong  string = `
  The 'keyserver remove' command allows to remove a defined keyserver from a specific
  endpoint. If no endpoint is specified, it will use the default remote endpoint (SylabsCloud).`
	KeyserverRemoveExample string = `
  $ singularity keyserver remove https://keys.example.com`
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	// keyserver list command
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	KeyserverListUse   string = `list`
	KeyserverListShort string = `List all keyservers that are configured`
	KeyserverListLong  string = `
  The 'keyserver list' command lists all keyservers configured for use.`
	KeyserverListExample string = `
  $ singularity remote list`
)
