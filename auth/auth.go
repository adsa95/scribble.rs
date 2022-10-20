package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt"
	"net/http"
	"time"
)

type User struct {
	Id   string
	Name string
}

type UserClaims struct {
	User
	jwt.StandardClaims
}

type Service struct {
	JwtKey        []byte
	JwtCookieName string
}

func (a Service) SetUserCookie(w http.ResponseWriter, user *User) error {
	tokenString, err := a.generateToken(user)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     a.JwtCookieName,
		Value:    tokenString,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(time.Hour * 12),
	})

	return nil
}

func (a Service) RemoveUserCookie(w http.ResponseWriter) error {
	http.SetCookie(w, &http.Cookie{
		Name:     a.JwtCookieName,
		Value:    "",
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
	})

	return nil
}

func (a Service) generateToken(user *User) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, UserClaims{
		User:           *user,
		StandardClaims: jwt.StandardClaims{},
	})

	return token.SignedString(a.JwtKey)
}

func (a Service) IsAuthenticated(r *http.Request) bool {
	user, _ := a.GetUser(r)
	return user != nil
}

func (a Service) GetUser(r *http.Request) (*User, error) {
	userCookie, cookieError := r.Cookie(a.JwtCookieName)
	if cookieError != nil {
		return nil, cookieError
	}

	token, jwtParseError := jwt.ParseWithClaims(userCookie.Value, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
		}

		return a.JwtKey, nil
	})
	if jwtParseError != nil {
		return nil, jwtParseError
	}

	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		return &claims.User, nil
	}

	return nil, fmt.Errorf("unknown JWT parsing error")
}

func (a *Service) CheckUser(handler func(w http.ResponseWriter, r *http.Request, user *User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.GetUser(r)
		if err != nil {
			handler(w, r, nil)
		} else {
			handler(w, r, user)
		}
	}
}

func (a Service) RequireUser(successHandler func(http.ResponseWriter, *http.Request, User), errorhandler func(http.ResponseWriter, *http.Request, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.GetUser(r)
		if err != nil {
			errorhandler(w, r, err)
			return
		}

		successHandler(w, r, *user)
	}
}
