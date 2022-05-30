package frontend

import (
	"github.com/julienschmidt/httprouter"
	"github.com/scribble-rs/scribble.rs/api"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/database"
	"github.com/scribble-rs/scribble.rs/state"
	"github.com/scribble-rs/scribble.rs/translations"
	"log"
	"net/http"
)

type JoinHandler struct {
	db *database.DB
}

type JoinPageData struct {
	*AuthenticatedBasePageData
	Translation translations.Translation
	Locale      string
}

func NewJoinPageData(user *auth.User) *JoinPageData {
	return &JoinPageData{
		AuthenticatedBasePageData: NewAuthenticatedBasePageData(api.RootPath, user),
	}
}

func (h JoinHandler) ssrJoinForm(w http.ResponseWriter, r *http.Request, u *auth.User) {
	translation, locale := determineTranslation(r)
	pageData := NewJoinPageData(u)
	pageData.Translation = translation
	pageData.Locale = locale

	err := pageTemplates.ExecuteTemplate(w, "join-page", pageData)
	if err != nil {
		log.Println(err.Error())
	}
}

func (h JoinHandler) join(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())
	username := params.ByName("username")

	if username == "" {
		userFacingError(w, "No username provided")
		return
	}

	lobbyId, err := h.db.GetLastLobbyForUser(username)
	if err != nil {
		userFacingError(w, "User or lobby not found")
		return
	}

	lobby := state.GetLobby(lobbyId)
	if lobby == nil {
		userFacingError(w, "User or lobby not found")
		return
	}

	http.Redirect(w, r, "/lobbies/"+lobby.LobbyID+"/play", http.StatusFound)
}
