// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
package singularity

const (
	userServicePath = "/v1/rbac/users/current"
)

type userOidcMeta struct {
	Secret string `json:"secret"`
}

type userData struct {
	OidcMeta userOidcMeta `json:"oidc_user_meta"`
	Email    string       `json:"email"`
	Realname string       `json:"realname"`
	Username string       `json:"username"`
}
