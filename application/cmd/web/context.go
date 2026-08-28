package main

//create this new type, so that we can
//distinguish context keys made by this application
//and not have naming collisions with third party packages
type contextKey string

const isAuthenticatedContextKey = contextKey("isAuthenticated")
const isActiveContextKey = contextKey("isActive")
const isAdminContextKey = contextKey("isAdmin")
const userNameContextKey = contextKey("userName")
const playerIDContextKey = contextKey("playerID")
const teamIDsContextKey = contextKey("teamIDs")
const captainTeamIDsContextKey = contextKey("captainTeamIDs")
const scorekeeperTeamIDsContextKey = contextKey("scorekeeperTeamIDs")
const leagueAdminLeagueIDsContextKey = contextKey("leagueAdminLeagueIDs")
const leagueAdminTeamIDsContextKey = contextKey("leagueAdminTeamIDs")

// realIsAdminContextKey always reflects the account's true admin status,
// even while viewingAsPlayerContextKey is suppressing isAdminContextKey —
// so an admin who's toggled into player view can still find their way
// back. See "view as player" in middleware.go's authenticate.
const realIsAdminContextKey = contextKey("realIsAdmin")
const viewingAsPlayerContextKey = contextKey("viewingAsPlayer")
