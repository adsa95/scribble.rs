package game

import (
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/twitch"
)

type Service struct {
	Twitch *twitch.Client
}

func (g *Service) CanJoin(user *auth.User, lobby *Lobby) (bool, string, error) {
	if !lobby.HasFreePlayerSlot() {
		return false, "lobby is full", nil
	}

	if lobby.HasBeenKicked(user) {
		return false, "kicked", nil
	}

	if lobby.RequireFollow {
		followEntry, err := g.Twitch.CheckUserFollows(&user.Tokens, user.Id, lobby.Owner.ID)
		if err != nil {
			return false, "", err
		}
		if followEntry == nil {
			return false, "must be following " + lobby.Owner.Name, nil
		}
	}

	if lobby.RequireSubscribed {
		subEntry, err := g.Twitch.CheckUserSubscription(&user.Tokens, user.Id, lobby.Owner.ID)
		if err != nil {
			return false, "", err
		}
		if subEntry == nil {
			return false, "must be subscribed to " + lobby.Owner.Name, nil
		}
	}

	banEntry, err := g.Twitch.CheckUserBanned(&lobby.Owner.GetUser().Tokens, user.Id, lobby.Owner.ID)
	if err != nil {
		return false, "", err
	} else if banEntry != nil {
		return false, "banned", nil
	}

	return true, "", nil
}
