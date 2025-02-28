package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/scribble-rs/scribble.rs/auth"
	"log"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/scribble-rs/scribble.rs/game"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func wsLobbyEndpoint(w http.ResponseWriter, r *http.Request, user auth.User) {
	lobby, lobbyError := GetLobby(r)
	if lobbyError != nil {
		http.Error(w, lobbyError.Error(), http.StatusNotFound)
		return
	}

	lobby.Synchronized(func() {
		player := lobby.GetPlayer(&user)
		if player == nil {
			http.Error(w, "you don't have access to this lobby;usersession unknown", http.StatusUnauthorized)
			return
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[INFO] %s connected to lobby %s", player, lobby)

		player.SetWebsocket(ws)
		lobby.OnPlayerConnectUnsynchronized(player)

		ws.SetCloseHandler(func(code int, text string) error {
			lobby.OnPlayerDisconnect(player)
			return nil
		})

		go wsListenToPlayer(lobby, player, ws)
	})
}

func wsListenToPlayer(lobby *game.Lobby, player *game.Player, socket *websocket.Conn) {
	//Workaround to prevent crash, since not all kind of
	//disconnect errors are cleanly caught by gorilla websockets.
	defer func() {
		err := recover()
		if err != nil {
			log.Printf("Error occurred in wsListen.\n\tError: %s\n\tPlayer: %s(%s)\nStack %s\n", err, player.Name, player.ID, string(debug.Stack()))
			lobby.OnPlayerDisconnect(player)
		}
	}()

	for {
		messageType, data, err := socket.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err) || websocket.IsUnexpectedCloseError(err) ||
				//This happens when the server closes the connection. It will cause 1000 retries followed by a panic.
				strings.Contains(err.Error(), "use of closed network connection") {
				//Make sure that the sockethandler is called
				lobby.OnPlayerDisconnect(player)
				//If the error is fatal, we stop listening for more messages.
				return
			}

			log.Printf("Error reading from socket: %s\n", err)
			//If the error doesn't seem fatal we attempt listening for more messages.
			continue
		}

		if messageType == websocket.TextMessage {
			received := &game.GameEvent{}
			err := json.Unmarshal(data, received)
			if err != nil {
				log.Printf("Error unmarshalling message: %s\n", err)
				sendError := WriteJSON(player.SocketConnection, game.GameEvent{Type: "system-message", Data: fmt.Sprintf("An error occurred trying to read your request, please report the error via GitHub: %s!", err)})
				if sendError != nil {
					log.Printf("Error sending errormessage: %s\n", sendError)
				}
				continue
			}

			handleError := lobby.HandleEvent(data, received, player)
			if handleError != nil {
				log.Printf("Error handling event: %s\n", handleError)
			}
		}
	}
}

func wsObserveEndpoint(w http.ResponseWriter, r *http.Request) {
	lobby, lobbyError := GetLobby(r)
	if lobbyError != nil {
		http.Error(w, lobbyError.Error(), http.StatusNotFound)
		return
	}

	lobby.Synchronized(func() {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Println("Anonymous observer has connected")

		observer := lobby.JoinObserver()
		observer.SetWebsocket(ws)
		lobby.OnObserverConnectUnsynchronized(observer)

		ws.SetCloseHandler(func(code int, text string) error {
			lobby.OnObserverDisconnect(observer)
			return nil
		})

		go wsListenToObserver(lobby, observer, ws)
	})
}

func wsListenToObserver(lobby *game.Lobby, observer *game.Observer, socket *websocket.Conn) {
	//Workaround to prevent crash, since not all kind of
	//disconnect errors are cleanly caught by gorilla websockets.
	defer func() {
		err := recover()
		if err != nil {
			log.Printf("Error occurred in wsListen.\n\tError: %s\n\tStack %s\n", err, string(debug.Stack()))
			lobby.OnObserverDisconnect(observer)
		}
	}()

	for {
		_, _, err := socket.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err) || websocket.IsUnexpectedCloseError(err) ||
				//This happens when the server closes the connection. It will cause 1000 retries followed by a panic.
				strings.Contains(err.Error(), "use of closed network connection") {
				//Make sure that the sockethandler is called
				lobby.OnObserverDisconnect(observer)
				//If the error is fatal, we stop listening for more messages.
				return
			}

			log.Printf("Error reading from socket: %s\n", err)
			//If the error doesn't seem fatal we attempt listening for more messages.
			continue
		}
	}
}

// WriteJSON marshals the given input into a JSON string and sends it to the
// player using the currently established websocket connection.
func WriteJSON(player *game.SocketConnection, object interface{}) error {
	player.GetWebsocketMutex().Lock()
	defer player.GetWebsocketMutex().Unlock()

	socket := player.GetWebsocket()
	if socket == nil || !player.Connected {
		return errors.New("player not connected")
	}

	return socket.WriteJSON(object)
}
