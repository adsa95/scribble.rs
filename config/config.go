package config

import (
	"log"
	"os"
	"strings"
)

type UrlGeneratorFunc func(path string) string

type Config struct {
	Host               string
	JwtKey             string
	JwtCookieName      string
	TwitchClientId     string
	TwitchClientSecret string
	TwitchRedirectURI  string
	DatabaseUrl        string
	GenerateUrl        UrlGeneratorFunc
}

func FromEnv() Config {
	rootUrl, rootUrlSet := os.LookupEnv("ROOT_URL")
	dbUrl, dbUrlSet := os.LookupEnv("DATABASE_URL")
	jwtKey, jwtKeySet := os.LookupEnv("JWT_KEY")
	jwtCookieName, jwtCookieNameSet := os.LookupEnv("JWT_COOKIE_NAME")
	twitchClientId, twitchClientIdSet := os.LookupEnv("TWITCH_CLIENT_ID")
	twitchClientSecret, twitchClientSecretSet := os.LookupEnv("TWITCH_CLIENT_SECRET")
	twitchRedirectURI, twitchRedirectURISet := os.LookupEnv("TWITCH_REDIRECT_URI")

	if !rootUrlSet {
		log.Fatalln("ROOT_URL not set")
	} else if !dbUrlSet {
		log.Fatalln("DATABASE_URL not set")
	} else if !jwtKeySet {
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
		DatabaseUrl:        dbUrl,
		GenerateUrl: func(path string) string {
			return strings.TrimSuffix(rootUrl, "/") + "/" + strings.TrimPrefix(path, "/")
		},
	}
}
