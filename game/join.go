package game

import (
	"fmt"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/twitch"
)

type Service struct {
	Twitch *twitch.Client
	Tokens twitch.TokenStore
}

func (g *Service) CanJoin(user *auth.User, lobby *Lobby) (bool, string, error) {
	if !lobby.HasFreePlayerSlot() {
		return false, "lobby is full", nil
	}

	if lobby.HasBeenKicked(user) {
		return false, "kicked", nil
	}

	userTokens, err := g.Tokens.Get(user)
	if err != nil {
		return false, "", err
	}

	if lobby.RequireFollow {
		if userTokens == nil {
			return false, "", fmt.Errorf(
				"no tokens for user %s (%s), can't check follow status",
				user.Name,
				user.Id,
			)
		}

		followEntry, err := g.Twitch.CheckUserFollows(userTokens, user.Id, lobby.Owner.ID)
		if err != nil {
			return false, "", err
		}
		if followEntry == nil {
			return false, "must be following " + lobby.Owner.Name, nil
		}
	}

	if lobby.RequireSubscribed {
		if userTokens == nil {
			return false, "", fmt.Errorf(
				"no tokens for user %s (%s), can't check follow status",
				user.Name,
				user.Id,
			)
		}

		subEntry, err := g.Twitch.CheckUserSubscription(userTokens, user.Id, lobby.Owner.ID)
		if err != nil {
			return false, "", err
		}
		if subEntry == nil {
			return false, "must be subscribed to " + lobby.Owner.Name, nil
		}
	}

	owner := lobby.Owner.GetUser()
	ownerTokens, err := g.Tokens.Get(owner)
	if err != nil {
		return false, "", err
	} else if ownerTokens == nil {
		return false, "", fmt.Errorf(
			"no tokens for lobby owner %s (%s), can't check ban status for %s (%s)",
			owner.Name,
			owner.Id,
			user.Name,
			user.Id,
		)
	}

	banEntry, err := g.Twitch.CheckUserBanned(ownerTokens, user.Id, lobby.Owner.ID)
	if err != nil {
		return false, "", err
	} else if banEntry != nil {
		return false, "banned", nil
	}

	return true, "", nil
}
