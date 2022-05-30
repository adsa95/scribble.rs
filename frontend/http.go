package frontend

import (
	"embed"
	"github.com/julienschmidt/httprouter"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/config"
	"github.com/scribble-rs/scribble.rs/database"
	"github.com/scribble-rs/scribble.rs/twitch"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/scribble-rs/scribble.rs/api"
	"github.com/scribble-rs/scribble.rs/translations"
)

var (
	//go:embed templates/*
	templateFS    embed.FS
	pageTemplates *template.Template

	//go:embed resources/*
	frontendResourcesFS embed.FS
)

//In this init hook we initialize all templates that could at some point
//be needed during the server runtime. If any of the templates can't be
//loaded, we panic.
func init() {
	var templateParseError error
	pageTemplates, templateParseError = template.ParseFS(templateFS, "templates/*")
	if templateParseError != nil {
		panic(templateParseError)
	}
}

var currentBasePageConfig = &BasePageConfig{
	RootPath: api.RootPath,
}

// BasePageConfig is data that all pages require to function correctly, no matter
// whether error page or lobby page.
type BasePageConfig struct {
	// RootPath is the path directly after the domain and before the
	// scribble.rs paths. For example if you host scribblers on painting.com
	// but already host a different website, then your API paths might have to
	// look like this: painting.com/scribblers/v1.
	RootPath string `json:"rootPath"`
}

// SetupRoutes registers the official webclient endpoints with the router.
func SetupRoutes(generateUrl config.UrlGeneratorFunc, r *httprouter.Router, a *auth.Service, t *twitch.Client, db *database.DB) {
	authHandler := &AuthHandler{
		db:           db,
		authService:  a,
		twitchClient: t,
		generateUrl:  generateUrl,
	}

	createHandler := &CreateHandler{
		db: db,
	}

	settingsHandler := &SettingsHandler{
		db:          db,
		twitch:      t,
		generateUrl: generateUrl,
	}

	joinHandler := &JoinHandler{
		db: db,
	}

	r.HandlerFunc("GET", "/", a.CheckUser(joinHandler.ssrJoinForm))
	r.HandlerFunc("GET", "/join/:username", joinHandler.join)

	r.HandlerFunc("GET", "/login", authHandler.ssrLogin)
	r.HandlerFunc("GET", "/logout", authHandler.ssrLogout)
	r.HandlerFunc("GET", "/login_twitch_callback", authHandler.ssrTwitchCallback)

	r.HandlerFunc("GET", "/lobbies", requireUserOrRedirect(a, createHandler.ssrCreateForm))
	r.HandlerFunc("POST", "/lobbies", requireUserOrRedirect(a, createHandler.ssrCreateLobby))
	r.HandlerFunc("GET", "/lobbies/:lobbyId/play", requireUserOrRedirect(a, ssrEnterLobby))
	r.HandlerFunc("GET", "/lobbies/:lobbyId/observe", ssrObserveLobby)

	r.HandlerFunc("GET", "/settings", requireUserOrRedirect(a, settingsHandler.ssrSettings))
	r.HandlerFunc("GET", "/settings_twitch_callback", requireUserOrRedirect(a, settingsHandler.ssrTwitchCallback))

	r.Handler("GET", "/resources/*path", http.StripPrefix(api.RootPath, http.FileServer(http.FS(frontendResourcesFS))))
}

// errorPageData represents the data that error.html requires to be displayed.
type errorPageData struct {
	*BasePageConfig
	// ErrorMessage displayed on the page.
	ErrorMessage string

	Translation translations.Translation
	Locale      string
}

//userFacingError will return the occurred error as a custom html page to the caller.
func userFacingError(w http.ResponseWriter, errorMessage string) {
	err := pageTemplates.ExecuteTemplate(w, "error-page", &errorPageData{
		BasePageConfig: currentBasePageConfig,
		ErrorMessage:   errorMessage,
	})
	//This should never happen, but if it does, something is very wrong.
	if err != nil {
		panic(err)
	}
}

func generalUserFacingError(w http.ResponseWriter) {
	userFacingError(w, "Something went wrong")
}

func requireUserOrRedirect(a *auth.Service, h func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return a.RequireUser(h, loginPageRedirect)
}

func loginPageRedirect(w http.ResponseWriter, r *http.Request, e error) {
	params := url.Values{}
	params.Add("intended", strings.TrimPrefix(r.URL.String(), api.RootPath))

	http.Redirect(w, r, api.RootPath+"/login?"+params.Encode(), http.StatusFound)
}
