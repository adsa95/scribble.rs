package frontend

import (
	"embed"
	"github.com/julienschmidt/httprouter"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/config"
	"github.com/scribble-rs/scribble.rs/database"
	"github.com/scribble-rs/scribble.rs/game"
	"github.com/scribble-rs/scribble.rs/twitch"
	"html/template"
	"log"
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
func SetupRoutes(generateUrl config.UrlGeneratorFunc, r *httprouter.Router, a *auth.Service, t *twitch.Client, db *database.DB, g *game.Service, tokens twitch.TokenStore) {
	authHandler := &AuthHandler{
		db:           db,
		authService:  a,
		twitchClient: t,
		generateUrl:  generateUrl,
		tokens:       tokens,
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

	lobbyHandler := &LobbyHandler{
		gameService: g,
	}

	requireScopeMiddleware := RequireScopeMiddleware{
		auth:        a,
		twitch:      t,
		generateUrl: generateUrl,
		tokens:      tokens,
	}

	r.HandlerFunc("GET", "/", a.CheckUser(joinHandler.ssrJoinForm))
	r.HandlerFunc("GET", "/join/:username", joinHandler.join)

	r.HandlerFunc("GET", "/login", authHandler.ssrLogin)
	r.HandlerFunc("GET", "/logout", authHandler.ssrLogout)
	r.HandlerFunc("GET", "/login_twitch_callback", authHandler.ssrTwitchCallback)

	r.HandlerFunc("GET", "/lobbies", requireScopeMiddleware.Handler([]string{"user:read:subscriptions", "moderation:read"}, createHandler.ssrCreateForm))
	r.HandlerFunc("POST", "/lobbies", requireScopeMiddleware.Handler([]string{"user:read:subscriptions", "moderation:read"}, createHandler.ssrCreateLobby))
	r.HandlerFunc("GET", "/lobbies/:lobbyId/play", requireScopeMiddleware.Handler([]string{"user:read:subscriptions"}, lobbyHandler.ssrEnterLobby))
	r.HandlerFunc("GET", "/lobbies/:lobbyId/observe", lobbyHandler.ssrObserveLobby)

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

type RequireScopeMiddleware struct {
	auth        *auth.Service
	twitch      *twitch.Client
	tokens      twitch.TokenStore
	generateUrl config.UrlGeneratorFunc
}

func (m *RequireScopeMiddleware) Handler(scopes []string, nextHandler func(w http.ResponseWriter, r *http.Request, user auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := m.auth.GetUser(r)
		if err != nil {
			m.redirectAuth(w, r, scopes)
			return
		}

		tokens, err := m.tokens.Get(user)
		if err != nil {
			log.Printf("[frontend/http][ERR] Failed getting tokens for user %s: %v", user, err)
			userFacingError(w, "an error occurred")
			return
		} else if tokens == nil {
			m.redirectAuth(w, r, scopes)
			return
		}

		for _, requiredScope := range scopes {
			if !tokens.HasScope(requiredScope) {
				combinedScopes := append(tokens.Scopes, scopes...)
				m.redirectAuth(w, r, combinedScopes)
				return
			}
		}

		nextHandler(w, r, *user)
	}
}

func (m *RequireScopeMiddleware) redirectAuth(w http.ResponseWriter, r *http.Request, scopes []string) {
	authUrl := m.twitch.GetAuthURI(m.generateUrl("/login_twitch_callback"), strings.TrimPrefix(r.URL.String(), api.RootPath), &scopes)
	http.Redirect(w, r, authUrl, http.StatusFound)
}

func loginPageRedirect(w http.ResponseWriter, r *http.Request, e error) {
	params := url.Values{}
	params.Add("intended", strings.TrimPrefix(r.URL.String(), api.RootPath))

	http.Redirect(w, r, api.RootPath+"/login?"+params.Encode(), http.StatusFound)
}
