package frontend

import (
	"github.com/scribble-rs/scribble.rs/api"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/translations"
	"github.com/scribble-rs/scribble.rs/twitch"
	"net/http"
)

type AuthHandler struct {
	authService  auth.Service
	twitchClient twitch.Client
}

func (h AuthHandler) ssrLogin(w http.ResponseWriter, r *http.Request) {
	var intended *string = nil
	if r.URL.Query().Has("intended") {
		s := r.URL.Query().Get("intended")
		intended = &s
	}

	var authURI = h.twitchClient.GetAuthURI(intended, nil)
	http.Redirect(w, r, authURI, http.StatusFound)
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

	twitchUser, verificationError := h.twitchClient.GetUserFromCode(r.URL.Query().Get("code"))
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

	redirectPath := "/"
	if r.URL.Query().Has("state") {
		redirectPath = api.RootPath + r.URL.Query().Get("state")
	}
	http.Redirect(w, r, redirectPath, http.StatusFound)
}
