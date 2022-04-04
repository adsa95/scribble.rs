package api

import (
	"github.com/scribble-rs/scribble.rs/auth"
	"net/http"
	"os"
)

// RootPath is the path directly after the domain and before the
// scribble.rs paths. For example if you host scribblers on painting.com
// but already host a different website, then your API paths might have to
// look like this: painting.com/scribblers/v1.
var RootPath string

//In this init hook we initialize all templates that could at some point
//be needed during the server runtime. If any of the templates can't be
//loaded, we panic.
func init() {
	rootPath, rootPathAvailable := os.LookupEnv("ROOT_PATH")
	if rootPathAvailable && rootPath != "" {
		RootPath = rootPath
	}
}

// SetupRoutes registers the /v1/ endpoints with the http package.
func SetupRoutes(authService auth.Service) {
	http.HandleFunc(RootPath+"/v1/stats", statsEndpoint)
	//The websocket is shared between the public API and the official client
	http.HandleFunc(RootPath+"/v1/ws", authService.RequireUser(wsEndpoint, HttpUnauthorized))

	//These exist only for the public API. We version them in order to ensure
	//backwards compatibility as far as possible.
	http.HandleFunc(RootPath+"/v1/lobby", authService.RequireUser(lobbyEndpoint, HttpUnauthorized))
	http.HandleFunc(RootPath+"/v1/lobby/player", authService.RequireUser(enterLobbyEndpoint, HttpUnauthorized))
}

func requireOrUnauthorized(a auth.Service, h func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return a.RequireUser(h, HttpUnauthorized)
}

func HttpUnauthorized(w http.ResponseWriter, r *http.Request, e error) {
	http.Error(w, e.Error(), http.StatusUnauthorized)
}
