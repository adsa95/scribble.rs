package main

import (
	"log"
	"os"
)

type Config struct {
	Host               string
	JwtKey             string
	JwtCookieName      string
	TwitchClientId     string
	TwitchClientSecret string
	TwitchRedirectURI  string
}

func ConfigFromEnv() Config {
	jwtKey, jwtKeySet := os.LookupEnv("JWT_KEY")
	jwtCookieName, jwtCookieNameSet := os.LookupEnv("JWT_COOKIE_NAME")
	twitchClientId, twitchClientIdSet := os.LookupEnv("TWITCH_CLIENT_ID")
	twitchClientSecret, twitchClientSecretSet := os.LookupEnv("TWITCH_CLIENT_SECRET")
	twitchRedirectURI, twitchRedirectURISet := os.LookupEnv("TWITCH_REDIRECT_URI")

	if !jwtKeySet {
		log.Fatalln("JWT_KEY not set")
	} else if !twitchClientIdSet {
		log.Fatalln("TWITCH_CLIENT_ID not set")
	} else if !twitchClientSecretSet {
		log.Fatalln("TWITCH_CLIENT_SECRET not set")
	}
	if !jwtCookieNameSet {
		jwtCookieName = "usertoken"
	}
	if !twitchRedirectURISet {
		twitchRedirectURI = "http://localhost:8080/login_twitch_callback"
	}

	return Config{
		JwtKey:             jwtKey,
		JwtCookieName:      jwtCookieName,
		TwitchClientId:     twitchClientId,
		TwitchClientSecret: twitchClientSecret,
		TwitchRedirectURI:  twitchRedirectURI,
	}
}
