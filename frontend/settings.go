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

type SettingsHandler struct {
	db          *database.DB
	twitch      *twitch.Client
	generateUrl config.UrlGeneratorFunc
	tokens      twitch.TokenStore
}

type settingsPageData struct {
	*AuthenticatedBasePageData
	Translation   translations.Translation
	Locale        string
	Mods          *[]database.UserDigest
	SyncTwitchUrl string
}

func (h *SettingsHandler) ssrSettings(w http.ResponseWriter, r *http.Request, u auth.User) {
	mods, err := h.db.GetModsForChannel(u.Id)
	if err != nil {
		generalUserFacingError(w)
		return
	}

	translation, locale := determineTranslation(r)

	pageData := settingsPageData{
		AuthenticatedBasePageData: NewAuthenticatedBasePageData(api.RootPath, &u),
		Translation:               translation,
		Locale:                    locale,
		Mods:                      mods,
		SyncTwitchUrl:             h.generateUrl("/settings/sync"),
	}

	templateErr := pageTemplates.ExecuteTemplate(w, "settings-page", pageData)
	if templateErr != nil {
		log.Println(templateErr.Error())
	}
}

func (h *SettingsHandler) syncTwitchModSettings(w http.ResponseWriter, r *http.Request, u auth.User) {
	tokens, err := h.tokens.Get(&u)
	if err != nil {
		generalUserFacingError(w)
		return
	}

	mods, getModsErr := h.twitch.GetAllModerators(tokens, u.Id)
	if getModsErr != nil {
		generalUserFacingError(w)
		return
	}

	setModErr := h.db.SetModsForChannel(u.Id, mods)
	if setModErr != nil {
		generalUserFacingError(w)
		return
	}

	http.Redirect(w, r, h.generateUrl("/settings"), http.StatusFound)
}
