package frontend

import (
	"github.com/scribble-rs/scribble.rs/api"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/translations"
	"github.com/scribble-rs/scribble.rs/twitch"
	"log"
	"net/http"
)

type AuthHandler struct {
	authService  auth.Service
	twitchClient twitch.Client
}

type loginPageData struct {
	BasePageConfig
	Translation    translations.Translation
	Locale         string
	TwitchLoginURI string
}

func (h AuthHandler) ssrLogin(w http.ResponseWriter, r *http.Request) {
	if h.authService.IsAuthenticated(r) {
		h.authService.RemoveUserCookie(w)
	}

	var intended *string = nil
	if r.URL.Query().Has("intended") {
		s := r.URL.Query().Get("intended")
		intended = &s
	}

	var authURI = h.twitchClient.GetAuthURI(intended, nil)

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

func (h AuthHandler) ssrLogout(w http.ResponseWriter, r *http.Request) {
	_ = h.authService.RemoveUserCookie(w)

	pageTemplates.ExecuteTemplate(w, "logged-out", &logoutPageData{
		BasePageConfig: BasePageConfig{
			RootPath: api.RootPath,
		},
	})
}

func (h AuthHandler) ssrTwitchCallback(w http.ResponseWriter, r *http.Request) {
	if !r.URL.Query().Has("code") {
		userFacingError(w, "No Twitch Code present in auth callback")
		return
	}

	twitchUser, _, verificationError := h.twitchClient.GetUserFromCode(r.URL.Query().Get("code"))
	if verificationError != nil {
		userFacingError(w, "Could not get user from Twitch Auth Code")
		return
	}

	user := auth.User{
		Id:         twitchUser.Id,
		TwitchName: twitchUser.DisplayName,
	}

	cookieError := h.authService.SetUserCookie(w, &user)
	if cookieError != nil {
		http.Error(w, cookieError.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully logged in user %v (%v)", user.TwitchName, user.Id)

	redirectPath := "/"
	if r.URL.Query().Has("state") {
		redirectPath = api.RootPath + r.URL.Query().Get("state")
	}
	http.Redirect(w, r, redirectPath, http.StatusFound)
}
