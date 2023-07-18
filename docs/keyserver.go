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
  The 'keyserver add' command lets the user specify an additional keyserver.
  The --order specifies the order of the new keyserver relative to the 
  keyservers that have already been specified. Therefore, when specifying
  '--order 1', the new keyserver will become the primary one. If no endpoint is
  specified, the new keyserver will be associated with the default remote
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
  The 'keyserver remove' command lets the user remove a previously specified
  keyserver from a specific endpoint. If no endpoint is specified, the default
  remote endpoint (SylabsCloud) will be assumed.`
	KeyserverRemoveExample string = `
  $ singularity keyserver remove https://keys.example.com`
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	// keyserver list command
	// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
	KeyserverListUse   string = `list [remoteName]`
	KeyserverListShort string = `List all keyservers that are configured`
	KeyserverListLong  string = `
  The 'keyserver list' command lists all keyservers configured for use with a
  given remote endpoint. If no endpoint is specified, the default
  remote endpoint (SylabsCloud) will be assumed.`
	KeyserverListExample string = `
  $ singularity keyserver list`
)
