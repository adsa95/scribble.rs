package twitch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Tokens struct {
	AccessToken  string
	RefreshToken string
	Scopes       []string
}

type User struct {
	Id              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	ProfileImageUrl string `json:"profile_image_url"`
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
		params.Add("scopes", strings.Join(*scopes, ","))
	}

	if state != nil {
		params.Add("state", *state)
	}

	return "https://id.twitch.tv/oauth2/authorize?" + params.Encode()
}

func (c Client) GetUserFromCode(code string) (*User, error) {
	token, tokenError := c.getAccessTokenFromCode(code)
	if tokenError != nil {
		return nil, tokenError
	}

	userResult, getUsersError := c.GetUsers(token, url.Values{})
	if getUsersError != nil {
		return nil, getUsersError
	}

	if len(userResult.Data) != 1 {
		return nil, fmt.Errorf("more or less than one user recieved, should be exactly one")
	}

	return &userResult.Data[0], nil
}

func (c Client) GetUsers(accessToken string, query url.Values) (*GetUsersResult, error) {
	request, newRequestError := http.NewRequest("GET", "https://api.twitch.tv/helix/users?"+query.Encode(), bytes.NewBuffer(make([]byte, 0)))
	if newRequestError != nil {
		return nil, newRequestError
	}

	request.Header.Set("Authorization", "Bearer "+accessToken)

	var result GetUsersResult
	err := c.doAndParseJson(request, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (c Client) getAccessTokenFromCode(code string) (string, error) {
	params := map[string]string{
		"client_id":     c.ClientId,
		"client_secret": c.ClientSecret,
		"code":          code,
		"grant_type":    "authorization_code",
		"redirect_uri":  c.RedirectURI,
	}

	request, newRequestError := http.NewRequest("POST", "https://id.twitch.tv/oauth2/token"+toQueryString(params), bytes.NewBuffer([]byte("")))
	if newRequestError != nil {
		return "", newRequestError
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}

	err := c.doAndParseJson(request, &result)
	if err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

func (c Client) doAndParseJson(r *http.Request, result any) error {
	client := http.Client{}

	r.Header.Set("Client-Id", c.ClientId)

	response, doError := client.Do(r)
	if doError != nil {
		return doError
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return fmt.Errorf("%v", response.Status)
	}

	bodyString, readError := io.ReadAll(response.Body)
	if readError != nil {
		return readError
	}

	decodeError := json.Unmarshal(bodyString, result)
	if decodeError != nil {
		return decodeError
	}

	return nil
}

func toQueryString(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	var res = "?"
	var first = true
	for key, value := range params {
		if !first {
			res = res + "&"
		}

		res = res + key + "=" + value

		if first {
			first = false
		}
	}

	return res
}
