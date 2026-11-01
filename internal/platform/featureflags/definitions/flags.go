// Package definitions declares provider-independent feature flag keys and fallbacks.
// Application code may reference these constants without depending on a provider SDK.
package definitions

const (
	// UsersListKey identifies the flag controlling user listing.
	UsersListKey = "users-list"
	// UsersListFallback keeps listing available when flag evaluation is unavailable.
	UsersListFallback = true
)
