package frontend

import (
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/game"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/scribble-rs/scribble.rs/api"
	"github.com/scribble-rs/scribble.rs/translations"
	"golang.org/x/text/language"
)

type LobbyHandler struct {
	gameService *game.Service
}

type lobbyPageData struct {
	*BasePageConfig
	*api.LobbyData

	Translation translations.Translation
	Locale      string
}

type observePageData struct {
	*lobbyPageData
	DelaySeconds int
}

type robotPageData struct {
	*BasePageConfig
	*api.LobbyData
}

func (h *LobbyHandler) ssrObserveLobby(w http.ResponseWriter, r *http.Request) {
	lobby, err := api.GetLobby(r)
	if err != nil {
		userFacingError(w, err.Error())
		return
	}

	userAgent := strings.ToLower(r.UserAgent())
	if !(strings.Contains(userAgent, "gecko") || strings.Contains(userAgent, "chrome") || strings.Contains(userAgent, "opera") || strings.Contains(userAgent, "safari")) {
		templatingError := pageTemplates.ExecuteTemplate(w, "robot-page", &robotPageData{
			BasePageConfig: currentBasePageConfig,
			LobbyData:      api.CreateLobbyData(lobby),
		})
		if templatingError != nil {
			log.Printf("error templating robot page: %d\n", templatingError)
		}
		return
	}

	translation, locale := determineTranslation(r)

	var delaySeconds int
	delaySeconds, _ = strconv.Atoi(r.URL.Query().Get("delay"))

	if delaySeconds < 0 {
		delaySeconds = 0
	}

	var pageData *observePageData
	lobby.Synchronized(func() {
		pageData = &observePageData{
			lobbyPageData: &lobbyPageData{
				BasePageConfig: currentBasePageConfig,
				LobbyData:      api.CreateLobbyData(lobby),
				Translation:    translation,
				Locale:         locale,
			},
			DelaySeconds: delaySeconds,
		}
	})

	//If the pagedata isn't initialized, it means the synchronized block has exited.
	//In this case we don't want to template the lobby, since an error has occurred
	//and probably already has been handled.
	if pageData != nil {
		templateError := pageTemplates.ExecuteTemplate(w, "lobby-observe-page", pageData)
		if templateError != nil {
			log.Printf("Error templating lobby: %s\n", templateError)
		}
	}
}

// ssrEnterLobby opens a lobby, either opening it directly or asking for a lobby.
func (h *LobbyHandler) ssrEnterLobby(w http.ResponseWriter, r *http.Request, u auth.User) {
	lobby, err := api.GetLobby(r)
	if err != nil {
		generalUserFacingError(w)
		return
	}

	userAgent := strings.ToLower(r.UserAgent())
	if !(strings.Contains(userAgent, "gecko") || strings.Contains(userAgent, "chrome") || strings.Contains(userAgent, "opera") || strings.Contains(userAgent, "safari")) {
		templatingError := pageTemplates.ExecuteTemplate(w, "robot-page", &robotPageData{
			BasePageConfig: currentBasePageConfig,
			LobbyData:      api.CreateLobbyData(lobby),
		})
		if templatingError != nil {
			log.Printf("error templating robot page: %d\n", templatingError)
		}
		return
	}

	translation, locale := determineTranslation(r)

	var pageData *lobbyPageData
	lobby.Synchronized(func() {
		player := lobby.GetPlayer(&u)

		if player == nil {
			canJoin, reason, err := h.gameService.CanJoin(&u, lobby)
			if err != nil {
				userFacingError(w, "An error occurred")
				return
			}

			if !canJoin {
				userFacingError(w, "You're not allowed to join: "+reason)
				return
			}

			lobby.JoinPlayer(&u)
		} else {
			if player.Connected && player.GetWebsocket() != nil {
				userFacingError(w, "It appears you already have an open tab for this lobby.")
				return
			}
		}

		pageData = &lobbyPageData{
			BasePageConfig: currentBasePageConfig,
			LobbyData:      api.CreateLobbyData(lobby),
			Translation:    translation,
			Locale:         locale,
		}
	})

	//If the pagedata isn't initialized, it means the synchronized block has exited.
	//In this case we don't want to template the lobby, since an error has occurred
	//and probably already has been handled.
	if pageData != nil {
		templateError := pageTemplates.ExecuteTemplate(w, "lobby-page", pageData)
		if templateError != nil {
			log.Printf("Error templating lobby: %s\n", templateError)
		}
	}
}

func determineTranslation(r *http.Request) (translations.Translation, string) {
	var translation translations.Translation

	languageTags, _, languageParseError := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if languageParseError == nil {
		for _, languageTag := range languageTags {
			fullLanguageIdentifier := languageTag.String()
			fullLanguageIdentifierLowercased := strings.ToLower(fullLanguageIdentifier)
			translation = translations.GetLanguage(fullLanguageIdentifierLowercased)
			if translation != nil {
				return translation, fullLanguageIdentifierLowercased
			}

			baseLanguageIdentifier, _ := languageTag.Base()
			baseLanguageIdentifierLowercased := strings.ToLower(baseLanguageIdentifier.String())
			translation = translations.GetLanguage(baseLanguageIdentifierLowercased)
			if translation != nil {
				return translation, baseLanguageIdentifierLowercased
			}
		}
	}

	return translations.DefaultTranslation, "en-us"
}
