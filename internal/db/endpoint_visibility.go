package db

import "strings"

const (
	EndpointVisibilityOwnerOnly      = "owner_only"
	EndpointVisibilitySelectedUsers  = "selected_users"
	EndpointVisibilityAllArcaUsers   = "all_arca_users"
	EndpointVisibilityInternetPublic = "internet_public"
)

func NormalizeEndpointVisibility(visibility string) string {
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case EndpointVisibilitySelectedUsers:
		return EndpointVisibilitySelectedUsers
	case EndpointVisibilityAllArcaUsers:
		return EndpointVisibilityAllArcaUsers
	case EndpointVisibilityInternetPublic:
		return EndpointVisibilityInternetPublic
	default:
		return EndpointVisibilityOwnerOnly
	}
}

func IsInternetPublicVisibility(visibility string) bool {
	return NormalizeEndpointVisibility(visibility) == EndpointVisibilityInternetPublic
}
