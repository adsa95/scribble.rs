package twitch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TokenSet struct {
	AccessToken          string
	RefreshToken         string
	FetchedAt            time.Time
	AccessTokenExpiresAt time.Time
	Scopes               []string
}

type User struct {
	Id              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	BroadcasterType string `json:"broadcaster_type"`
	Description     string `json:"description"`
	ProfileImageUrl string `json:"profile_image_url"`
	OfflineImageUrl string `json:"offline_image_url"`
	ViewCount       int    `json:"view_count"`
	CreatedAt       string `json:"created_at"`
}

type GetUsersResult struct {
	Data []User `json:"data"`
}

type Client struct {
	ClientId     string
	ClientSecret string
	RedirectURI  string
}

func (c Client) GetAuthURI(state *string, scopes *[]string) string {
	params := url.Values{}
	params.Add("client_id", c.ClientId)
	params.Add("redirect_uri", c.RedirectURI)
	params.Add("response_type", "code")

	if scopes != nil && len(*scopes) > 0 {
		params.Add("scope", strings.Join(*scopes, ","))
	}

	if state != nil {
		params.Add("state", *state)
	}

	return "https://id.twitch.tv/oauth2/authorize?" + params.Encode()
}

func (c Client) GetUserFromCode(code string) (*User, *TokenSet, error) {
	tokens, tokenError := c.getTokenSetFromCode(code)
	if tokenError != nil {
		return nil, nil, tokenError
	}

	userResult, getUsersError := c.GetUsers(tokens, url.Values{})
	if getUsersError != nil {
		return nil, nil, getUsersError
	}

	if len(userResult.Data) != 1 {
		return nil, nil, fmt.Errorf("more or less than one user recieved, should be exactly one")
	}

	return &userResult.Data[0], tokens, nil
}

func (c Client) GetUsers(tokens *TokenSet, query url.Values) (*GetUsersResult, error) {
	request, newRequestError := http.NewRequest("GET", "https://api.twitch.tv/helix/users?"+query.Encode(), bytes.NewBuffer(make([]byte, 0)))
	if newRequestError != nil {
		return nil, newRequestError
	}

	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)

	var result GetUsersResult
	err := c.doAndParseJson(request, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

type BannedUserEntry struct {
	UserId         string `json:"user_id"`
	UserLogin      string `json:"user_login"`
	UserName       string `json:"user_name"`
	ExpiresAt      string `json:"expires_at"`
	Reason         string `json:"reason"`
	ModeratorId    string `json:"moderator_id"`
	ModeratorLogin string `json:"moderator_login"`
	ModeratorName  string `json:"moderator_name"`
}

type GetBannedUsersResult struct {
	Data       []BannedUserEntry `json:"data"`
	Pagination struct {
		Cursor string `json:"cursor"`
	} `json:"pagination"`
}

func (c Client) GetAllBannedUsers(tokens *TokenSet, broadcasterId string) ([]BannedUserEntry, error) {
	res := make([]BannedUserEntry, 0)
	cursor := ""

	for true {
		r, err := c.GetBannedUsers(tokens, broadcasterId, cursor)
		if err != nil {
			return nil, err
		}

		res = append(res, r.Data...)

		if r.Pagination.Cursor != "" {
			cursor = r.Pagination.Cursor
		} else {
			break
		}
	}

	return res, nil
}

func (c Client) GetBannedUsers(tokens *TokenSet, broadcasterId string, cursor string) (*GetBannedUsersResult, error) {
	params := url.Values{}
	params.Add("first", "100")
	params.Add("broadcaster_id", broadcasterId)

	if cursor != "" {
		params.Add("cursor", cursor)
	}

	request, newRequestError := http.NewRequest("GET", "https://api.twitch.tv/helix/moderation/banned?"+params.Encode(), bytes.NewBuffer(make([]byte, 0)))
	if newRequestError != nil {
		return nil, newRequestError
	}

	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)

	var result GetBannedUsersResult
	err := c.doAndParseJson(request, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

type ModeratorEntry struct {
	UserId    string `json:"user_id"`
	UserLogin string `json:"user_login"`
	UserName  string `json:"user_name"`
}

type GetModeratorsResult struct {
	Data       []ModeratorEntry `json:"data"`
	Pagination struct {
		Cursor string `json:"cursor"`
	} `json:"pagination"`
}

func (c Client) GetAllModerators(tokens *TokenSet, broadcasterId string) ([]ModeratorEntry, error) {
	res := make([]ModeratorEntry, 0)
	cursor := ""

	for true {
		r, err := c.GetModerators(tokens, broadcasterId, cursor)
		if err != nil {
			return nil, err
		}

		res = append(res, r.Data...)

		if r.Pagination.Cursor != "" {
			cursor = r.Pagination.Cursor
		} else {
			break
		}
	}

	return res, nil
}

func (c Client) GetModerators(tokens *TokenSet, broadcasterId string, cursor string) (*GetModeratorsResult, error) {
	params := url.Values{}
	params.Add("broadcaster_id", broadcasterId)
	params.Add("first", "1")

	if cursor != "" {
		params.Add("cursor", cursor)
	}

	request, newRequestError := http.NewRequest("GET", "https://api.twitch.tv/helix/moderation/moderators?"+params.Encode(), bytes.NewBuffer(make([]byte, 0)))
	if newRequestError != nil {
		return nil, newRequestError
	}

	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)

	var result GetModeratorsResult
	err := c.doAndParseJson(request, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (c Client) getTokenSetFromCode(code string) (*TokenSet, error) {
	params := url.Values{}
	params.Set("client_id", c.ClientId)
	params.Set("client_secret", c.ClientSecret)
	params.Set("code", code)
	params.Set("grant_type", "authorization_code")
	params.Set("redirect_uri", c.RedirectURI)

	request, newRequestError := http.NewRequest("POST", "https://id.twitch.tv/oauth2/token?"+params.Encode(), bytes.NewBuffer([]byte("")))
	if newRequestError != nil {
		return nil, newRequestError
	}

	var result struct {
		AccessToken  string   `json:"access_token"`
		RefreshToken string   `json:"refresh_token"`
		ExpiresIn    int64    `json:"expires_in"`
		TokenType    string   `json:"token_type"`
		Scope        []string `json:"scope"`
	}

	err := c.doAndParseJson(request, &result)
	if err != nil {
		return nil, err
	}

	if result.TokenType != "bearer" {
		return nil, fmt.Errorf("invalid token type: %s", result.TokenType)
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(result.ExpiresIn) * time.Second)

	set := TokenSet{
		AccessToken:          result.AccessToken,
		RefreshToken:         result.RefreshToken,
		FetchedAt:            now,
		AccessTokenExpiresAt: expiresAt,
		Scopes:               []string{},
	}

	return &set, nil
}

func (c Client) doAndParseJson(r *http.Request, result any) error {
	client := http.Client{}

	r.Header.Set("Client-Id", c.ClientId)

	response, doError := client.Do(r)
	if doError != nil {
		return doError
	}
	defer response.Body.Close()

	bodyString, readError := io.ReadAll(response.Body)
	if readError != nil {
		return readError
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("%v", response.Status)
	}

	decodeError := json.Unmarshal(bodyString, result)
	if decodeError != nil {
		return decodeError
	}

	return nil
}
