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
	"net/url"
)

type SettingsHandler struct {
	db          *database.DB
	twitch      *twitch.Client
	generateUrl config.UrlGeneratorFunc
}

type settingsPageData struct {
	*BasePageConfig
	Translation   translations.Translation
	Locale        string
	Banned        *[]auth.User
	Mods          *[]auth.User
	SyncTwitchUrl string
}

func (h *SettingsHandler) ssrSettings(w http.ResponseWriter, r *http.Request, u auth.User) {
	banned, err := h.db.GetBannedForChannel(u.Id)
	if err != nil {
		generalUserFacingError(w)
		return
	}

	mods, err := h.db.GetModsForChannel(u.Id)
	if err != nil {
		generalUserFacingError(w)
		return
	}

	syncUrl := h.twitch.GetAuthURI(h.generateUrl("/settings_twitch_callback"), "", &[]string{"moderation:read"})

	translation, locale := determineTranslation(r)

	pageData := settingsPageData{
		BasePageConfig: &BasePageConfig{
			RootPath: api.RootPath,
		},
		Translation:   translation,
		Locale:        locale,
		Banned:        banned,
		Mods:          mods,
		SyncTwitchUrl: syncUrl,
	}

	templateErr := pageTemplates.ExecuteTemplate(w, "settings-page", pageData)
	if templateErr != nil {
		log.Println(templateErr.Error())
	}
}

func (h *SettingsHandler) ssrTwitchCallback(w http.ResponseWriter, r *http.Request, u auth.User) {
	tokens, tokenErr := h.twitch.GetTokenSetFromCode(r.URL.Query().Get("code"))
	if tokenErr != nil {
		generalUserFacingError(w)
		return
	}

	users, usersErr := h.twitch.GetUsers(tokens, url.Values{})
	if usersErr != nil {
		generalUserFacingError(w)
		return
	}

	if len(users.Data) != 1 {
		generalUserFacingError(w)
		return
	}

	user := users.Data[0]

	banned, getBannedErr := h.twitch.GetAllBannedUsers(tokens, user.Id)
	if getBannedErr != nil {
		generalUserFacingError(w)
		return
	}

	setBannedErr := h.db.SetBannedForChannel(user.Id, banned)
	if setBannedErr != nil {
		generalUserFacingError(w)
		return
	}

	mods, getModsErr := h.twitch.GetAllModerators(tokens, user.Id)
	if getModsErr != nil {
		generalUserFacingError(w)
		return
	}

	setModErr := h.db.SetModsForChannel(user.Id, mods)
	if setModErr != nil {
		generalUserFacingError(w)
		return
	}

	http.Redirect(w, r, h.generateUrl("/settings"), http.StatusFound)
}
