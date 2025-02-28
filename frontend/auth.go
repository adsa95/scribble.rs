package frontend

import (
	"github.com/scribble-rs/scribble.rs/api"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/config"
	"github.com/scribble-rs/scribble.rs/database"
	"github.com/scribble-rs/scribble.rs/translations"
	"github.com/scribble-rs/scribble.rs/twitch"
	"log"
	"net/http"
)

type AuthHandler struct {
	db           *database.DB
	authService  *auth.Service
	twitchClient *twitch.Client
	generateUrl  config.UrlGeneratorFunc
	tokens       twitch.TokenStore
}

type AuthenticatedBasePageData struct {
	*BasePageConfig
	User *auth.User
}

func NewAuthenticatedBasePageData(rootPath string, user *auth.User) *AuthenticatedBasePageData {
	return &AuthenticatedBasePageData{
		BasePageConfig: &BasePageConfig{
			RootPath: rootPath,
		},
		User: user,
	}
}

type loginPageData struct {
	BasePageConfig
	Translation    translations.Translation
	Locale         string
	TwitchLoginURI string
}

func (h *AuthHandler) ssrLogin(w http.ResponseWriter, r *http.Request) {
	if h.authService.IsAuthenticated(r) {
		h.authService.RemoveUserCookie(w)
	}

	intended := ""
	if r.URL.Query().Has("intended") {
		intended = r.URL.Query().Get("intended")
	}

	var authURI = h.twitchClient.GetAuthURI(h.generateUrl("/login_twitch_callback"), intended, &[]string{"user:read:subscriptions"})

	translation, locale := determineTranslation(r)
	templateError := pageTemplates.ExecuteTemplate(w, "login-page", &loginPageData{
		BasePageConfig: BasePageConfig{
			RootPath: api.RootPath,
		},
		Translation:    translation,
		Locale:         locale,
		TwitchLoginURI: authURI,
	})
	if templateError != nil {
		log.Println(templateError.Error())
	}
}

type logoutPageData struct {
	BasePageConfig
	Translation translations.Translation
	Locale      string
}

func (h *AuthHandler) ssrLogout(w http.ResponseWriter, r *http.Request) {
	_ = h.authService.RemoveUserCookie(w)

	pageTemplates.ExecuteTemplate(w, "logged-out", &logoutPageData{
		BasePageConfig: BasePageConfig{
			RootPath: api.RootPath,
		},
	})
}

func (h *AuthHandler) ssrTwitchCallback(w http.ResponseWriter, r *http.Request) {
	if !r.URL.Query().Has("code") {
		userFacingError(w, "No Twitch Code present in auth callback")
		return
	}

	twitchUser, userTokens, verificationError := h.twitchClient.GetUserFromCode(r.URL.Query().Get("code"))
	if verificationError != nil {
		userFacingError(w, "Could not get user from Twitch Auth Code")
		return
	}

	user := auth.User{
		Id:   twitchUser.Id,
		Name: twitchUser.DisplayName,
	}

	err := h.tokens.Set(&user, userTokens)
	if err != nil {
		log.Printf("[ERR][tokens] Failed setting tokens for user %s", user)
	}

	upsertError := h.db.UpsertUser(&user)
	if upsertError != nil {
		log.Printf("[ERR][DB] Failed upserting user %s: %v", user, upsertError)
	}

	cookieError := h.authService.SetUserCookie(w, &user)
	if cookieError != nil {
		http.Error(w, cookieError.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO][AUTH] %s logged in successfully", user)

	redirectPath := "/"
	if r.URL.Query().Has("state") {
		redirectPath = api.RootPath + r.URL.Query().Get("state")
	}
	http.Redirect(w, r, redirectPath, http.StatusFound)
}
