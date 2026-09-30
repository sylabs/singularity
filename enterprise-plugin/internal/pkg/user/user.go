// Copyright (c) 2021-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package user

import (
	"errors"
	"fmt"
	"sync"

	"github.com/sylabs/singularity/v4/enterprise-plugin/internal/pkg/api"
	userops "github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/client/users"
	"github.com/sylabs/singularity/v4/enterprise-plugin/pkg/consentservice/models"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const UnknownUser = "UNKNOWN"

var ErrUnknownUser = errors.New("unknown user")

var (
	userIDCache    map[string]string
	usernameCache  map[string]string
	cachePopulated sync.Once
)

func idToUser(id string, clients *api.Clients) (user *models.User, err error) {
	params := userops.NewGetUserParams().WithUserID(id)

	resp, err := clients.ConsentClient.Users.GetUser(params, nil)
	if err != nil {
		return nil, err
	}

	data := resp.GetPayload().Data
	return data, nil
}

// ID to Username returns the username for a given user BSON ID, if possible.
// If the username cannot be resolved, const UnknownUser is returned.
// This method is intended for displaying a friendly username instead of a BSON ID.
func IDToUsername(id string, clients *api.Clients) string {
	if userIDCache == nil {
		userIDCache = make(map[string]string)
	}

	if user, ok := userIDCache[id]; ok {
		return user
	}
	user, err := idToUser(id, clients)
	if err != nil {
		sylog.Warningf("Could not lookup username: %v", err)
		userIDCache[id] = UnknownUser
		return UnknownUser
	}

	userIDCache[id] = user.Username
	return user.Username
}

func populateUsernameCache(clients *api.Clients) error {
	usernameCache = make(map[string]string)

	resp, err := clients.ConsentClient.Users.ListUsers(userops.NewListUsersParams(), nil)
	if err != nil {
		return err
	}

	data := resp.GetPayload().Data

	for _, u := range data {
		usernameCache[u.Username] = u.ID
	}

	return nil
}

// UsernameToID translates a non-BSON ID string to the BSON ID of that user, if possible.
// If the provided string is a BSON ID already, it is returned directly.
// If the user ID for the provided username cannot be found an error is returned.
func UsernameToID(user string, clients *api.Clients) (uid string, err error) {
	_, err = bson.ObjectIDFromHex(user)
	if err == nil {
		return user, nil
	}

	// The only way we can do this is by listing all users, and then looking there,
	// so cache the user list the first time we are called.
	cachePopulated.Do(func() {
		err := populateUsernameCache(clients)
		if err != nil {
			sylog.Warningf("Could not retrieve username/ID mapping: %v", err)
		}
	})

	uid, ok := usernameCache[user]
	if !ok {
		return "", fmt.Errorf("%q - %w", user, ErrUnknownUser)
	}
	return uid, nil
}
