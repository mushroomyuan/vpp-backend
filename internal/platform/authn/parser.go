// Package authn defines authentication boundaries shared by transport adapters.
package authn

import "github.com/mushroomyuan/vpp-backend/platform/identity"

// PrincipalParser maps trusted ingress userinfo into the normalized identity contract.
type PrincipalParser func(userinfo string) (identity.Principal, error)
