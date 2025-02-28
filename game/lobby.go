package game

import (
	"errors"
	"fmt"
	"github.com/scribble-rs/scribble.rs/auth"
	"github.com/scribble-rs/scribble.rs/database"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mitchellh/mapstructure"
	"github.com/scribble-rs/scribble.rs/sanitize"

	discordemojimap "github.com/Bios-Marcel/discordemojimap/v2"
	"github.com/agnivade/levenshtein"
	"github.com/gofrs/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	LobbySettingBounds = &SettingBounds{
		MinDrawingTime: 60,
		MaxDrawingTime: 300,
		MinRounds:      1,
		MaxRounds:      20,
		MinMaxPlayers:  2,
		MaxMaxPlayers:  24,
	}
	SupportedLanguages = map[string]string{
		"english_gb": "English (GB)",
		"english":    "English (US)",
		"italian":    "Italian",
		"german":     "German",
		"french":     "French",
		"dutch":      "Dutch",
		"swedish":    "Swedish",
	}
)

const (
	DrawingBoardBaseWidth  = 1600
	DrawingBoardBaseHeight = 900
	MinBrushSize           = 8
	MaxBrushSize           = 32

	maxBaseScore      = 200
	maxHintBonusScore = 60
)

// SettingBounds defines the lower and upper bounds for the user-specified
// lobby creation input.
type SettingBounds struct {
	MinDrawingTime int64 `json:"minDrawingTime"`
	MaxDrawingTime int64 `json:"maxDrawingTime"`
	MinRounds      int64 `json:"minRounds"`
	MaxRounds      int64 `json:"maxRounds"`
	MinMaxPlayers  int64 `json:"minMaxPlayers"`
	MaxMaxPlayers  int64 `json:"maxMaxPlayers"`
}

// LineEvent is basically the same as GameEvent, but with a specific Data type.
// We use this for reparsing as soon as we know that the type is right. It's
// a bit unperformant, but will do for now.
type LineEvent struct {
	Type string `json:"type"`
	Data *Line  `json:"data"`
}

// FillEvent is basically the same as GameEvent, but with a specific Data type.
// We use this for reparsing as soon as we know that the type is right. It's
// a bit unperformant, but will do for now.
type FillEvent struct {
	Type string `json:"type"`
	Data *Fill  `json:"data"`
}

type Kick struct {
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
}

func (lobby *Lobby) HandleEvent(raw []byte, received *GameEvent, player *Player) error {
	if received.Type == "keep-alive" {
		//This is a known dummy event in order to avoid accidental websocket
		//connection closure. However, no action is required on the server.
		//Either way, we needn't needlessly lock the lobby.
		return nil
	}

	lobby.mutex.Lock()
	defer lobby.mutex.Unlock()

	if received.Type == "message" {
		dataAsString, isString := (received.Data).(string)
		if !isString {
			return fmt.Errorf("invalid data received: '%s'", received.Data)
		}

		handleMessage(dataAsString, player, lobby)
	} else if received.Type == "line" {
		if lobby.canDraw(player) {
			line := &Line{}
			//It's cheaper to restructure the already unmarshalled map data
			//instead of calling json.unmarshal again with a more specific
			//type. Benchmarks can be found in json_test.go.
			decodeError := mapstructure.Decode(received.Data, line)
			if decodeError != nil {
				return fmt.Errorf("error decoding data: %s", decodeError)
			}

			//In case the line is too big, we overwrite the data of the event.
			//This will prevent clients from lagging due to too thick lines.
			if line.LineWidth > float32(MaxBrushSize) {
				line.LineWidth = MaxBrushSize
			} else if line.LineWidth < float32(MinBrushSize) {
				line.LineWidth = MinBrushSize
			}

			now := time.Now()
			if now.Sub(lobby.lastDrawEvent) > 150*time.Millisecond || lobby.wasLastDrawEventFill() {
				lobby.connectedDrawEventsIndexStack = append(lobby.connectedDrawEventsIndexStack, len(lobby.currentDrawing))
			}
			lobby.lastDrawEvent = now

			lineEvent := &LineEvent{Type: "line", Data: line}
			lobby.AppendLine(lineEvent)

			//We directly forward the event, as it seems to be valid.
			lobby.sendDataToEveryoneExceptSender(player, lineEvent)
		}
	} else if received.Type == "fill" {
		if lobby.canDraw(player) {
			//It's cheaper to restructure the already unmarshalled map data
			//instead of calling json.unmarshal again with a more specific
			//type. Benchmarks can be found in json_test.go.
			fill := &Fill{}
			decodeError := mapstructure.Decode(received.Data, fill)
			if decodeError != nil {
				return fmt.Errorf("error decoding data: %s", decodeError)
			}

			//A fill always
			lobby.connectedDrawEventsIndexStack = append(lobby.connectedDrawEventsIndexStack, len(lobby.currentDrawing))
			lobby.lastDrawEvent = time.Now()

			lobby.AppendFill(&FillEvent{Type: "fill", Data: fill})

			//We directly forward the event, as it seems to be valid.
			lobby.sendDataToEveryoneExceptSender(player, received)
		}
	} else if received.Type == "clear-drawing-board" {
		if lobby.canDraw(player) && len(lobby.currentDrawing) > 0 {
			lobby.ClearDrawing()
			lobby.connectedDrawEventsIndexStack = nil
			lobby.sendDataToEveryoneExceptSender(player, received)
		}
	} else if received.Type == "undo" {
		if lobby.canDraw(player) && len(lobby.currentDrawing) > 0 && len(lobby.connectedDrawEventsIndexStack) > 0 {
			undoFrom := lobby.connectedDrawEventsIndexStack[len(lobby.connectedDrawEventsIndexStack)-1]
			lobby.connectedDrawEventsIndexStack = lobby.connectedDrawEventsIndexStack[:len(lobby.connectedDrawEventsIndexStack)-1]
			if undoFrom < len(lobby.currentDrawing) {
				lobby.currentDrawing = lobby.currentDrawing[:undoFrom]
				lobby.TriggerUpdateEvent("drawing", lobby.currentDrawing)
			}
		}
	} else if received.Type == "choose-word" {
		chosenIndex, isInt := (received.Data).(int)
		if !isInt {
			asFloat, isFloat64 := (received.Data).(float64)
			if !isFloat64 {
				return fmt.Errorf("invalid data in choose-word event: %v", received.Data)
			}

			chosenIndex = int(asFloat)
		}

		if len(lobby.wordChoice) == 0 {
			return errors.New("word was chosen, even though no choice was available")
		}

		if chosenIndex < 0 || chosenIndex >= len(lobby.wordChoice) {
			return fmt.Errorf("word choice was %d, but should've been >= 0 and < %d", chosenIndex, len(lobby.wordChoice))
		}

		if player == lobby.drawer {
			lobby.selectWord(chosenIndex)

			wordHintData := &GameEvent{Type: "update-wordhint", Data: lobby.wordHints}
			wordHintDataRevealed := &GameEvent{Type: "update-wordhint", Data: lobby.wordHintsShown}
			for _, otherPlayer := range lobby.GetPlayers() {
				if otherPlayer.State == Guessing {
					lobby.WriteJSON(otherPlayer.SocketConnection, wordHintData)
				} else {
					lobby.WriteJSON(otherPlayer.SocketConnection, wordHintDataRevealed)
				}
			}
			for _, observer := range lobby.GetObservers() {
				lobby.WriteJSON(observer.SocketConnection, wordHintData)
			}
		}
	} else if received.Type == "kick" {
		toKickID, isString := (received.Data).(string)
		if !isString {
			return fmt.Errorf("invalid data in kick event: %v", received.Data)
		}

		handleKickEvent(lobby, player, toKickID)
	} else if received.Type == "start" {
		if lobby.State != Ongoing && player == lobby.Owner {
			//We are reseting each players score, since players could
			//technically be player a second game after the last one
			//has already ended.
			for _, otherPlayer := range lobby.players {
				otherPlayer.Score = 0
				otherPlayer.LastScore = 0
				//Since nobody has any points in the beginning, everyone has practically
				//the same rank, therefore y'll winners for now.
				otherPlayer.Rank = 1
			}

			//Cause advanceLobby to start at round 1, starting the game anew.
			lobby.Round = 0

			advanceLobby(lobby)
		}
	} else if received.Type == "request-drawing" {
		//Since the client shouldn't be blocking to wait for the drawing, it's
		//fine to emit the event if there's no drawing.
		if len(lobby.currentDrawing) != 0 {
			lobby.WriteJSON(player.SocketConnection, GameEvent{Type: "drawing", Data: lobby.currentDrawing})
		}
	}

	return nil
}

func handleMessage(message string, sender *Player, lobby *Lobby) {
	//Very long message can cause lags and can therefore be easily abused.
	//While it is debatable whether a 10000 byte (not character) long
	//message makes sense, this is technically easy to manage and therefore
	//allowed for now.
	if len(message) > 10000 {
		return
	}

	trimmedMessage := strings.TrimSpace(message)
	//Empty message can neither be a correct guess nor are useful for
	//other players in the chat.
	if trimmedMessage == "" {
		return
	}

	//If no word is currently selected, all players can talk to each other
	//and we don't have to check for corrected guesses.
	if lobby.CurrentWord == "" {
		sendMessageToAll(trimmedMessage, sender, lobby)
		return
	}

	if sender.State != Guessing {
		lobby.sendMessageToAllNonGuessing(trimmedMessage, sender)
	} else {
		normInput := simplifyText(lobby.lowercaser.String(trimmedMessage))
		normSearched := simplifyText(lobby.CurrentWord)

		if normSearched == normInput {
			secondsLeft := int(lobby.RoundEndTime/1000 - time.Now().UTC().UnixNano()/1000000000)

			sender.LastScore = calculateGuesserScore(lobby.hintCount, lobby.hintsLeft, secondsLeft, lobby.DrawingTime)
			sender.Score += sender.LastScore

			lobby.scoreEarnedByGuessers += sender.LastScore
			sender.State = Standby

			lobby.TriggerUpdateEvent("correct-guess", sender.ID)

			if !lobby.isAnyoneStillGuessing() {
				advanceLobby(lobby)
			} else {
				//Since the word has been guessed correctly, we reveal it.
				lobby.WriteJSON(sender.SocketConnection, GameEvent{Type: "update-wordhint", Data: lobby.wordHintsShown})
				recalculateRanks(lobby)
				lobby.triggerPlayersUpdate()
			}
		} else if levenshtein.ComputeDistance(normInput, normSearched) == 1 {
			//In cases of a close guess, we still send the message to everyone.
			//This allows other players to guess the word by watching what the
			//other players are misstyping.
			sendMessageToAll(trimmedMessage, sender, lobby)
			lobby.WriteJSON(sender.SocketConnection, GameEvent{Type: "close-guess", Data: trimmedMessage})
		} else {
			sendMessageToAll(trimmedMessage, sender, lobby)
		}
	}
}

func (lobby *Lobby) wasLastDrawEventFill() bool {
	if len(lobby.currentDrawing) == 0 {
		return false
	}
	_, isFillEvent := lobby.currentDrawing[len(lobby.currentDrawing)-1].(*FillEvent)
	return isFillEvent
}

func calculateGuesserScore(hintCount, hintsLeft, secondsLeft, drawingTime int) int {
	//The base score is based on the general time taken.
	//The formula here represents an exponential decline based on the time taken.
	//This way fast players get more points, however not a lot more.
	//The bonus gained by guessing before hints are shown is therefore still somewhat relevant.
	declineFactor := 1.0 / float64(drawingTime)
	baseScore := int(maxBaseScore * math.Pow(1.0-declineFactor, float64(drawingTime-secondsLeft)))

	//Prevent zero division panic. This could happen with two letter words.
	if hintCount <= 0 {
		return baseScore
	}

	//If all hints are shown, or the word is too short to show hints, the
	//calculation will basically always be baseScore + 0.
	return baseScore + hintsLeft*(maxHintBonusScore/hintCount)
}

func (lobby *Lobby) isAnyoneStillGuessing() bool {
	for _, otherPlayer := range lobby.players {
		if otherPlayer.State == Guessing && otherPlayer.Connected {
			return true
		}
	}

	return false
}

func sendMessageToAll(message string, sender *Player, lobby *Lobby) {
	messageEvent := GameEvent{Type: "message", Data: Message{
		Author:   sender.Name,
		AuthorID: sender.ID,
		Content:  discordemojimap.Replace(message),
	}}
	for _, player := range lobby.players {
		lobby.WriteJSON(player.SocketConnection, messageEvent)
	}
	for _, observer := range lobby.observers {
		lobby.WriteJSON(observer.SocketConnection, messageEvent)
	}
}

func (lobby *Lobby) sendMessageToAllNonGuessing(message string, sender *Player) {
	messageEvent := GameEvent{Type: "non-guessing-player-message", Data: Message{
		Author:   sender.Name,
		AuthorID: sender.ID,
		Content:  discordemojimap.Replace(message),
	}}
	for _, target := range lobby.players {
		if target.State != Guessing {
			lobby.WriteJSON(target.SocketConnection, messageEvent)
		}
	}
}

func handleKickEvent(lobby *Lobby, player *Player, toKickID string) {
	//Kicking yourself isn't allowed
	if toKickID == player.ID {
		return
	}

	//Only lobby creator or mods can kick
	if player.ID != lobby.creator.ID && !lobby.IsMod(player.user) {
		return
	}

	playerToKickIndex := -1
	for index, otherPlayer := range lobby.players {
		if otherPlayer.ID == toKickID {
			playerToKickIndex = index
			break
		}
	}

	//If we haven't found the player, we can't kick them.
	if playerToKickIndex == -1 {
		return
	}

	playerToKick := lobby.players[playerToKickIndex]

	//Only the lobby creator can kick other mods
	if lobby.IsMod(playerToKick.user) && player.ID != lobby.creator.ID {
		return
	}

	kickEvent := &GameEvent{
		Type: "kick",
		Data: &Kick{
			PlayerID:   playerToKick.ID,
			PlayerName: playerToKick.Name,
		},
	}

	//We send the kick event to all players
	for _, otherPlayer := range lobby.players {
		lobby.WriteJSON(otherPlayer.SocketConnection, kickEvent)
	}
	for _, observer := range lobby.observers {
		lobby.WriteJSON(observer.SocketConnection, kickEvent)
	}

	kickPlayer(lobby, playerToKick, playerToKickIndex)

	log.Printf("[INFO] %s kicked by %s from %s", player, playerToKick, lobby)
}

// kickPlayer kicks the given player from the lobby, updating the lobby
// state and sending all necessary events.
func kickPlayer(lobby *Lobby, playerToKick *Player, playerToKickIndex int) {
	//Avoiding nilpointer in case playerToKick disconnects during this event unluckily.
	playerToKickSocket := playerToKick.ws
	if playerToKickSocket != nil {
		disconnectError := playerToKickSocket.Close()
		if disconnectError != nil {
			log.Printf("Error disconnecting kicked player:\n\t%s\n", disconnectError)
		}
	}

	//If the owner is kicked, we choose the next best person as the owner.
	if lobby.Owner == playerToKick {
		for _, otherPlayer := range lobby.players {
			potentialOwner := otherPlayer
			if potentialOwner.Connected {
				lobby.Owner = potentialOwner
				lobby.TriggerUpdateEvent("owner-change", &OwnerChangeEvent{
					PlayerID:   potentialOwner.ID,
					PlayerName: potentialOwner.Name,
				})
				break
			}
		}
	}

	lobby.KickedUsers = append(lobby.KickedUsers, *playerToKick.user)

	if lobby.drawer == playerToKick {
		newDrawer, roundOver := determineNextDrawer(lobby)
		lobby.players = append(lobby.players[:playerToKickIndex], lobby.players[playerToKickIndex+1:]...)
		lobby.TriggerUpdateEvent("drawer-kicked", nil)

		//Since the drawer has been kicked, that probably means that they were
		//probably probably trolling, therefore we redact everyones last earned
		//score.
		for _, otherPlayer := range lobby.players {
			otherPlayer.Score -= otherPlayer.LastScore
			otherPlayer.LastScore = 0
		}
		lobby.scoreEarnedByGuessers = 0
		lobby.drawer = nil

		advanceLobbyPredefineDrawer(lobby, roundOver, newDrawer)
	} else {
		lobby.players = append(lobby.players[:playerToKickIndex], lobby.players[playerToKickIndex+1:]...)

		if lobby.isAnyoneStillGuessing() {
			//This isn't necessary in case we need to advanced the lobby, as it has
			//to happen anyways and sending events twice would be wasteful.
			recalculateRanks(lobby)
			lobby.triggerPlayersUpdate()
		} else {
			advanceLobby(lobby)
		}
	}
}

type OwnerChangeEvent struct {
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
}

// advanceLobbyPredefineDrawer is required in cases where the drawer is removed game
func advanceLobbyPredefineDrawer(lobby *Lobby, roundOver bool, newDrawer *Player) {
	if lobby.timeLeftTicker != nil {
		//We want to create a new ticker later on. By setting the current
		//ticker to nil, we'll cause the ticker routine to stop the ticker
		//and then stop itself. Later on we create a new routine.
		//This way we won't have race conditions or wrongly executed logic.
		lobby.timeLeftTicker = nil
	}

	//The drawer can potentially be null if kicked or the game just started.
	if lobby.drawer != nil {
		if lobby.scoreEarnedByGuessers <= 0 {
			lobby.drawer.LastScore = 0
		} else {
			//Average score, but minus one player, since the own score is 0 and doesn't count.
			playerCount := lobby.GetConnectedPlayerCount()
			//If the drawer isn't connected though, we mustn't subtract from the count.
			if lobby.drawer.Connected {
				playerCount--
			}

			var averageScore int
			if playerCount > 0 {
				averageScore = lobby.scoreEarnedByGuessers / playerCount
			}

			lobby.drawer.LastScore = averageScore
			lobby.drawer.Score += lobby.drawer.LastScore
		}
	}

	if lobby.CurrentWord != "" {
		sendTurnOver(lobby, lobby.CurrentWord)
		time.Sleep(5 * time.Second)
	}

	lobby.CurrentWord = ""
	lobby.wordHints = nil

	if lobby.DrawingTimeNew != 0 {
		lobby.DrawingTime = lobby.DrawingTimeNew
	}
	lobby.scoreEarnedByGuessers = 0

	for _, otherPlayer := range lobby.players {
		//If the round ends and people still have guessing, that means the
		//"LastScore" value for the next turn has to be "no score earned".
		if otherPlayer.State == Guessing {
			otherPlayer.LastScore = 0
		}
		//Initially all players are in guessing state, as the drawer gets
		//defined further at the bottom.
		otherPlayer.State = Guessing
	}

	recalculateRanks(lobby)

	if roundOver {
		//Game over
		if lobby.Round == lobby.Rounds {
			lobby.drawer = nil
			lobby.State = GameOver

			for _, player := range lobby.players {
				readyData := generatePlayerReadyData(lobby, player)
				//The drawing is always available on the client, as the
				//game-over event is only sent to already connected players.
				readyData.CurrentDrawing = nil

				lobby.WriteJSON(player.SocketConnection, GameEvent{
					Type: "game-over",
					Data: &GameOverEvent{
						PlayerReady: readyData,
					}})
			}

			//Omit rest of events, since we don't need to advance.
			return
		}

		lobby.Round++
	}

	lobby.ClearDrawing()
	lobby.connectedDrawEventsIndexStack = nil
	lobby.drawer = newDrawer
	lobby.drawer.State = Drawing
	lobby.State = Ongoing
	lobby.wordChoice = GetRandomWords(3, lobby)

	//We use milliseconds for higher accuracy
	lobby.RoundEndTime = time.Now().UTC().UnixNano()/1000000 + int64(lobby.DrawingTime)*1000
	lobby.timeLeftTicker = time.NewTicker(1 * time.Second)
	go startTurnTimeTicker(lobby, lobby.timeLeftTicker)

	lobby.TriggerUpdateEvent("next-turn", &NextTurn{
		Round:        lobby.Round,
		Players:      lobby.players,
		RoundEndTime: int(lobby.RoundEndTime - getTimeAsMillis()),
	})

	lobby.WriteJSON(lobby.drawer.SocketConnection, &GameEvent{Type: "your-turn", Data: lobby.wordChoice})
}

type TurnOverEvent struct {
	Word   string             `json:"word"`
	Result []TurnPlayerResult `json:"result"`
}

type TurnPlayerResult struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

func sendTurnOver(lobby *Lobby, word string) {
	TurnResult := make([]TurnPlayerResult, len(lobby.players))

	for i, player := range lobby.players {
		TurnResult[i] = TurnPlayerResult{
			Name:  player.Name,
			Score: player.LastScore,
		}
	}

	lobby.TriggerUpdateEvent("turn-over", &TurnOverEvent{
		Word:   word,
		Result: TurnResult,
	})
}

// advanceLobby will either start the game or jump over to the next turn.
func advanceLobby(lobby *Lobby) {
	newDrawer, roundOver := determineNextDrawer(lobby)
	advanceLobbyPredefineDrawer(lobby, roundOver, newDrawer)
}

// GameOverEvent is basically the ready event, but contains the last word.
// This is required in order to show the last player the word, in case they
// didn't manage to guess it in time. This is necessary since the last word
// is usually part of the "next-turn" event, which we don't send, since the
// game is over already.
type GameOverEvent struct {
	*PlayerReady
}

// determineNextDrawer returns the next person that's supposed to be drawing, but
// doesn't tell the lobby yet. The boolean signals whether the current round
// is over.
func determineNextDrawer(lobby *Lobby) (*Player, bool) {
	for index, player := range lobby.players {
		if player == lobby.drawer {
			//If we have someone that's drawing, take the next one
			for i := index + 1; i < len(lobby.players); i++ {
				player := lobby.players[i]
				if player.Connected {
					return player, false
				}
			}

			//No player below the current drawer has been found, therefore we
			//fallback to our default logic at the bottom.
			break
		}
	}

	//We prefer the first connected player.
	for _, player := range lobby.players {
		if player.Connected {
			return player, true
		}
	}

	//If no player is connected, we simply chose the first player.
	//Safe, since the lobby can't be empty, as leaving doesn't remove players
	//from the array, but only sets them to a disconnected state.
	return lobby.players[0], true
}

// startTurnTimeTicker executes a loop that listens to the lobbies
// timeLeftTicker and executes a tickLogic on each tick. This method
// blocks until the turn ends.
func startTurnTimeTicker(lobby *Lobby, ticker *time.Ticker) {
	for {
		<-ticker.C
		if !lobby.tickLogic(ticker) {
			break
		}
	}
}

// tickLogic checks whether the lobby needs to proceed to the next round and
// updates the available word hints if required. The return value indicates
// whether additional ticks are necessary or not. The ticker is automatically
// stopped if no additional ticks are required.
func (lobby *Lobby) tickLogic(expectedTicker *time.Ticker) bool {
	lobby.mutex.Lock()
	defer lobby.mutex.Unlock()

	//Since we have a lock on the lobby, we can find out if the ticker we are
	//listening to is still valid. If not, we want to kill the outer routine.
	if lobby.timeLeftTicker != expectedTicker {
		expectedTicker.Stop()
		return false
	}

	currentTime := getTimeAsMillis()
	if currentTime >= lobby.RoundEndTime {
		expectedTicker.Stop()
		advanceLobby(lobby)
		//Kill outer goroutine and therefore avoid executing hint logic.
		return false
	}

	if lobby.hintsLeft > 0 && lobby.wordHints != nil {
		revealHintEveryXMilliseconds := int64(lobby.DrawingTime * 1000 / (lobby.hintCount + 1))
		//If you have a drawingtime of 120 seconds and three hints, you
		//want to reveal a hint every 40 seconds, so that the two hints
		//are visible for at least a third of the time. //If the word
		//was chosen at 60 seconds, we'll still reveal one hint
		//instantly, as the time is already lower than 80.
		revealHintAtXOrLower := revealHintEveryXMilliseconds * int64(lobby.hintsLeft)
		timeLeft := lobby.RoundEndTime - currentTime
		if timeLeft <= revealHintAtXOrLower {
			lobby.hintsLeft--

			//We are trying til we find a yet unshown wordhint. Since we have
			//thread safety and have already checked that there's a hint
			//left, this loop can never spin forever.
			for {
				randomIndex := rand.Int() % len(lobby.wordHints)
				if lobby.wordHints[randomIndex].Character == 0 {
					lobby.wordHints[randomIndex].Character = []rune(lobby.CurrentWord)[randomIndex]
					wordHintData := &GameEvent{Type: "update-wordhint", Data: lobby.wordHints}
					for _, otherPlayer := range lobby.GetPlayers() {
						if otherPlayer.State == Guessing {
							lobby.WriteJSON(otherPlayer.SocketConnection, wordHintData)
						}
					}
					for _, observer := range lobby.GetObservers() {
						lobby.WriteJSON(observer.SocketConnection, wordHintData)
					}
					break
				}
			}
		}
	}

	return true
}

func getTimeAsMillis() int64 {
	return time.Now().UTC().UnixNano() / 1000000
}

type TurnOver struct {
	Round      int    `json:"round"`
	RevealWord string `json:"revealWord"`
}

// NextTurn represents the data necessary for displaying the lobby state right
// after a new turn started. Meaning that no word has been chosen yet and
// therefore there are no wordhints and no current drawing instructions.
type NextTurn struct {
	Round        int       `json:"round"`
	Players      []*Player `json:"players"`
	RoundEndTime int       `json:"roundEndTime"`
}

// recalculateRanks will assign each player his respective rank in the lobby
// according to everyones current score. This will not trigger any events.
func recalculateRanks(lobby *Lobby) {
	//We don't directly sort the players, since the order determines in which
	//order the players will have to draw.
	sortedPlayers := make([]*Player, len(lobby.players))
	copy(sortedPlayers, lobby.players)
	sort.Slice(sortedPlayers, func(a, b int) bool {
		return sortedPlayers[a].Score > sortedPlayers[b].Score
	})

	//We start at maxint32, since we want the first player to cause an
	//increment of the the score, which will always happen this way, as
	//no player can have a score this high.
	lastScore := math.MaxInt32
	var lastRank int
	for _, player := range sortedPlayers {
		if !player.Connected {
			continue
		}

		if player.Score < lastScore {
			lastRank++
			player.Rank = lastRank
			lastScore = player.Score
		} else {
			//Since the players are already sorted from high to low, we only
			//have the cases higher or equal.
			player.Rank = lastRank
		}
	}
}

func (lobby *Lobby) selectWord(wordChoiceIndex int) {
	lobby.CurrentWord = lobby.wordChoice[wordChoiceIndex]
	lobby.wordChoice = nil

	//Depending on how long the word is, a fixed amount of hints
	//would be too easy or too hard.
	runeCount := utf8.RuneCountInString(lobby.CurrentWord)
	if runeCount <= 2 {
		lobby.hintCount = 0
	} else if runeCount <= 4 {
		lobby.hintCount = 1
	} else if runeCount <= 9 {
		lobby.hintCount = 2
	} else {
		lobby.hintCount = 3
	}
	lobby.hintsLeft = lobby.hintCount

	//We generate both the "empty" word hints and the hints for the
	//drawer. Since the length is the same, we do it in one run.
	lobby.wordHints = make([]*WordHint, 0, runeCount)
	lobby.wordHintsShown = make([]*WordHint, 0, runeCount)

	for _, char := range lobby.CurrentWord {
		//These characters are part of the word, but aren't relevant for the
		//guess. In order to make the word hints more useful to the
		//guesser, those are always shown. An example would be "Pac-Man".
		//Because these characters aren't relevant for the guess, they
		//aren't being underlined.
		isAlwaysVisibleCharacter := char == ' ' || char == '_' || char == '-'

		//The hints for the drawer are always visible, therefore they
		//don't require any handling of different cases.
		lobby.wordHintsShown = append(lobby.wordHintsShown, &WordHint{
			Character: char,
			Underline: !isAlwaysVisibleCharacter,
		})

		if isAlwaysVisibleCharacter {
			lobby.wordHints = append(lobby.wordHints, &WordHint{
				Character: char,
				Underline: false,
			})
		} else {
			lobby.wordHints = append(lobby.wordHints, &WordHint{
				Underline: true,
			})
		}
	}
}

func (lobby *Lobby) sendDataToEveryoneExceptSender(sender *Player, data interface{}) {
	for _, otherPlayer := range lobby.GetPlayers() {
		if otherPlayer != sender {
			lobby.WriteJSON(otherPlayer.SocketConnection, data)
		}
	}

	for _, observer := range lobby.GetObservers() {
		lobby.WriteJSON(observer.SocketConnection, data)
	}
}

func (lobby *Lobby) TriggerUpdateEvent(eventType string, data interface{}) {
	event := &GameEvent{Type: eventType, Data: data}
	for _, otherPlayer := range lobby.GetPlayers() {
		lobby.WriteJSON(otherPlayer.SocketConnection, event)
	}

	for _, observer := range lobby.GetObservers() {
		lobby.WriteJSON(observer.SocketConnection, event)
	}
}

func (lobby *Lobby) triggerPlayersUpdate() {
	lobby.TriggerUpdateEvent("update-players", lobby.players)
}

// CreateLobby creates a new lobby including the initial player (owner) and
// optionally returns an error, if any occurred during creation.
func CreateLobby(db *database.DB, user *auth.User, chosenLanguage string, publicLobby bool, drawingTime, rounds, maxPlayers, customWordsChance int, customWords []string, followersOnly bool, subsOnly bool) (*Player, *Lobby, error) {
	lobby := &Lobby{
		LobbyID: uuid.Must(uuid.NewV4()).String(),
		EditableLobbySettings: &EditableLobbySettings{
			Rounds:            rounds,
			DrawingTime:       drawingTime,
			MaxPlayers:        maxPlayers,
			CustomWordsChance: customWordsChance,
			Public:            publicLobby,
		},
		CustomWords:       customWords,
		currentDrawing:    make([]interface{}, 0),
		State:             Unstarted,
		db:                db,
		mutex:             &sync.Mutex{},
		RequireFollow:     followersOnly,
		RequireSubscribed: subsOnly,
	}

	if len(customWords) > 1 {
		rand.Shuffle(len(lobby.CustomWords), func(i, j int) {
			lobby.CustomWords[i], lobby.CustomWords[j] = lobby.CustomWords[j], lobby.CustomWords[i]
		})
	}

	lobby.Wordpack = chosenLanguage

	//Neccessary to correctly treat words from player, however, custom words might be treated incorrectly.
	lobby.lowercaser = cases.Lower(language.Make(getLanguageIdentifier(chosenLanguage)))

	//customWords are lowercased afterwards, as they are direct user input.
	if len(customWords) > 0 {
		for customWordIndex, customWord := range customWords {
			customWords[customWordIndex] = lobby.lowercaser.String(customWord)
		}
	}

	player := createPlayer(user, true)

	lobby.players = append(lobby.players, player)
	lobby.Owner = player
	lobby.creator = player

	return player, lobby, nil
}

// Message represents a message in the chatroom.
type Message struct {
	// Author is the player / thing that wrote the message
	Author string `json:"author"`
	// AuthorID is the unique identifier of the authors player object.
	AuthorID string `json:"authorId"`
	// Content is the actual message text.
	Content string `json:"content"`
}

// PlayerReady represents the initial state that a user needs upon connection.
// This includes all the necessary things for properly running a client
// without receiving any more data.
type PlayerReady struct {
	PlayerID     string `json:"playerId"`
	PlayerName   string `json:"playerName"`
	AllowDrawing bool   `json:"allowDrawing"`

	ObserverReady
}

type ObserverReady struct {
	VotekickEnabled    bool          `json:"votekickEnabled"`
	GameState          gameState     `json:"gameState"`
	OwnerID            string        `json:"ownerId"`
	Round              int           `json:"round"`
	Rounds             int           `json:"rounds"`
	RoundEndTime       int           `json:"roundEndTime"`
	DrawingTimeSetting int           `json:"drawingTimeSetting"`
	WordHints          []*WordHint   `json:"wordHints"`
	Players            []*Player     `json:"players"`
	CurrentDrawing     []interface{} `json:"currentDrawing"`
}

func generatePlayerReadyData(lobby *Lobby, player *Player) *PlayerReady {
	ready := &PlayerReady{
		PlayerID:     player.ID,
		AllowDrawing: player.State == Drawing,
		PlayerName:   player.Name,

		ObserverReady: ObserverReady{
			GameState:          lobby.State,
			OwnerID:            lobby.Owner.ID,
			Round:              lobby.Round,
			Rounds:             lobby.Rounds,
			DrawingTimeSetting: lobby.DrawingTime,
			WordHints:          lobby.GetAvailableWordHints(player.State),
			Players:            lobby.players,
			CurrentDrawing:     lobby.currentDrawing,
		},
	}

	if lobby.State != Ongoing {
		//Clients should interpret 0 as "time over", unless the gamestate isn't "ongoing"
		ready.RoundEndTime = 0
	} else {
		ready.RoundEndTime = int(lobby.RoundEndTime - getTimeAsMillis())
	}

	return ready
}

func generateObserverReadyData(lobby *Lobby) *ObserverReady {
	ready := &ObserverReady{
		GameState:          lobby.State,
		OwnerID:            lobby.Owner.ID,
		Round:              lobby.Round,
		Rounds:             lobby.Rounds,
		DrawingTimeSetting: lobby.DrawingTime,
		WordHints:          lobby.GetAvailableWordHints(Standby),
		Players:            lobby.players,
		CurrentDrawing:     lobby.currentDrawing,
	}

	if lobby.State != Ongoing {
		//Clients should interpret 0 as "time over", unless the gamestate isn't "ongoing"
		ready.RoundEndTime = 0
	} else {
		ready.RoundEndTime = int(lobby.RoundEndTime - getTimeAsMillis())
	}

	return ready
}

func (lobby *Lobby) OnObserverConnectUnsynchronized(observer *Observer) {
	observer.Connected = true
	lobby.WriteJSON(observer.SocketConnection, GameEvent{Type: "ready", Data: generateObserverReadyData(lobby)})
}

func (lobby *Lobby) OnObserverDisconnect(observer *Observer) {
	//We want to avoid calling the handler twice.
	if observer.ws == nil {
		return
	}

	observer.Connected = false
	observer.ws = nil
}

func (lobby *Lobby) OnPlayerConnectUnsynchronized(player *Player) {
	player.Connected = true
	recalculateRanks(lobby)
	lobby.WriteJSON(player.SocketConnection, GameEvent{Type: "ready", Data: generatePlayerReadyData(lobby, player)})

	//This state is reached if the player reconnects before having chosen a word.
	//This can happen if the player refreshes his browser page or the socket
	//loses connection and reconnects quickly.
	if lobby.drawer == player && lobby.CurrentWord == "" {
		lobby.WriteJSON(lobby.drawer.SocketConnection, &GameEvent{Type: "your-turn", Data: lobby.wordChoice})
	}

	event := &GameEvent{Type: "update-players", Data: lobby.players}
	for _, otherPlayer := range lobby.GetPlayers() {
		//The player that just joined already has the most up-to-date data due
		//to the ready event being sent. Therefeore it'd be wasteful to send
		//that player and update event for players.
		if otherPlayer != player {
			lobby.WriteJSON(otherPlayer.SocketConnection, event)
		}
	}
	for _, observer := range lobby.GetObservers() {
		lobby.WriteJSON(observer.SocketConnection, event)
	}
}

func (lobby *Lobby) OnPlayerDisconnect(player *Player) {
	//We want to avoid calling the handler twice.
	if player.ws == nil {
		return
	}

	disconnectTime := time.Now()

	//It is important to properly disconnect the player before aqcuiring the mutex
	//in order to avoid false assumptions about the players connection state
	//and avoid attempting to send events.
	log.Printf("[INFO] %s disconnected from %s", player, lobby)
	player.Connected = false
	player.ws = nil

	lobby.mutex.Lock()
	defer lobby.mutex.Unlock()

	player.disconnectTime = &disconnectTime
	lobby.LastPlayerDisconnectTime = &disconnectTime

	recalculateRanks(lobby)
	if lobby.hasConnectedPlayersInternal() {
		lobby.triggerPlayersUpdate()
	}
}

// GetAvailableWordHints returns a WordHint array depending on the players
// game state, since people that are drawing or have already guessed correctly
// can see all hints.
func (lobby *Lobby) GetAvailableWordHints(state PlayerState) []*WordHint {
	//The draw simple gets every character as a word-hint. We basically abuse
	//the hints for displaying the word, instead of having yet another GUI
	//element that wastes space.
	if state == Drawing || state == Standby {
		return lobby.wordHintsShown
	} else {
		return lobby.wordHints
	}
}

// JoinPlayer creates a new player object using the given name and adds it
// to the lobbies playerlist. The new players is returned.
func (lobby *Lobby) JoinPlayer(user *auth.User) *Player {
	player := createPlayer(user, lobby.IsMod(user))

	lobby.players = append(lobby.players, player)

	return player
}

func (lobby *Lobby) JoinObserver() *Observer {
	observer := CreateObserver()

	lobby.observers = append(lobby.observers, observer)

	return observer
}

func (lobby *Lobby) LeaveObserver(observer *Observer) {
	for i, ob := range lobby.observers {
		if ob == observer {
			lobby.observers = append(lobby.observers[:i], lobby.observers[i+1:]...)
			return
		}
	}
}

func (lobby *Lobby) canDraw(player *Player) bool {
	return lobby.drawer == player && lobby.CurrentWord != ""
}

var connectionCharacterReplacer = strings.NewReplacer(" ", "", "-", "", "_", "")

// simplifyText prepares the string for a more lax comparison of two words.
// Spaces, dashes, underscores and accented characters are removed or replaced.
func simplifyText(s string) string {
	return connectionCharacterReplacer.
		Replace(sanitize.ReplaceAccentedCharacters(s))
}

// Shutdown sends all players an event, indicating that the lobby
// will be shut down. The caller of this function should take care of not
// allowing new connections. Clients should gracefully disconnect.
func (lobby *Lobby) Shutdown() {
	lobby.mutex.Lock()
	defer lobby.mutex.Unlock()

	shutdownEvent := GameEvent{Type: "shutdown"}
	for _, player := range lobby.players {
		lobby.WriteJSON(player.SocketConnection, shutdownEvent)
	}
}
