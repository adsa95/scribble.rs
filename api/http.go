package api

import (
	"github.com/julienschmidt/httprouter"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/database"
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

// SetupRoutes registers the /api/v1/ endpoints with the router.
func SetupRoutes(r *httprouter.Router, a *auth.Service, db *database.DB) {
	handler := &Handler{Db: db}

	// We version the API in order to ensure
	// backwards compatibility as far as possible.
	apiPrefix := "/api/v1"
	apiRouter := httprouter.New()

	apiRouter.HandlerFunc("GET", "/stats", handler.statsEndpoint)

	//The websocket is shared between the public API and the official client
	apiRouter.HandlerFunc("GET", "/lobbies/:lobbyId/ws/play", requireUserOrUnauthorized(a, wsLobbyEndpoint))
	apiRouter.HandlerFunc("GET", "/lobbies/:lobbyId/ws/observe", wsObserveEndpoint)

	//These exist only for the public API.
	apiRouter.HandlerFunc("GET", "/lobbies/:lobbyId", requireUserOrUnauthorized(a, handler.lobbyEndpoint))
	apiRouter.HandlerFunc("GET", "/lobbies/:lobbyId/player", requireUserOrUnauthorized(a, handler.enterLobbyEndpoint))

	r.Handler("GET", apiPrefix+"/*path", http.StripPrefix(apiPrefix, apiRouter))
	r.Handler("POST", apiPrefix+"/*path", http.StripPrefix(apiPrefix, apiRouter))
}

func requireUserOrUnauthorized(a *auth.Service, h func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return a.RequireUser(h, HttpUnauthorized)
}

func HttpUnauthorized(w http.ResponseWriter, r *http.Request, e error) {
	http.Error(w, e.Error(), http.StatusUnauthorized)
}
