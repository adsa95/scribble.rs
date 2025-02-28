package game

import (
	"github.com/scribble-rs/scribble.rs/auth"
	"sync"
	"testing"
)

func Test_RemoveAccents(t *testing.T) {
	t.Run("Check removing accented characters", func(test *testing.T) {
		var expectedResults = map[string]string{
			"é":           "e",
			"É":           "E",
			"à":           "a",
			"À":           "A",
			"ç":           "c",
			"Ç":           "C",
			"ö":           "oe",
			"Ö":           "OE",
			"œ":           "oe",
			"\n":          "\n",
			"\t":          "\t",
			"\r":          "\r",
			"":            "",
			"·":           "·",
			"?:!":         "?:!",
			"ac-ab":       "acab",
			"ac - _ab-- ": "acab",
		}

		for k, v := range expectedResults {
			result := simplifyText(k)
			if result != v {
				t.Errorf("Error. Char was %s, but should've been %s", result, v)
			}
		}
	})
}

func Test_simplifyText(t *testing.T) {
	//We only test the replacement we do ourselves. We won't test
	//the "sanitize", or furthermore our expectations of it for now.

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "dash",
			input: "-",
			want:  "",
		},
		{
			name:  "underscore",
			input: "_",
			want:  "",
		},
		{
			name:  "space",
			input: " ",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifyText(tt.input); got != tt.want {
				t.Errorf("simplifyText() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_recalculateRanks(t *testing.T) {
	lobby := &Lobby{
		mutex: &sync.Mutex{},
	}
	lobby.players = append(lobby.players, &Player{
		ID:               "a",
		Score:            1,
		SocketConnection: &SocketConnection{Connected: true},
	})
	lobby.players = append(lobby.players, &Player{
		ID:               "b",
		Score:            1,
		SocketConnection: &SocketConnection{Connected: true},
	})
	recalculateRanks(lobby)

	rankPlayerA := lobby.players[0].Rank
	rankPlayerB := lobby.players[1].Rank
	if rankPlayerA != 1 || rankPlayerB != 1 {
		t.Errorf("With equal score, ranks should be equal. (A: %d; B: %d)",
			rankPlayerA, rankPlayerB)
	}

	lobby.players = append(lobby.players, &Player{
		ID:               "c",
		Score:            0,
		SocketConnection: &SocketConnection{Connected: true},
	})
	recalculateRanks(lobby)

	rankPlayerA = lobby.players[0].Rank
	rankPlayerB = lobby.players[1].Rank
	if rankPlayerA != 1 || rankPlayerB != 1 {
		t.Errorf("With equal score, ranks should be equal. (A: %d; B: %d)",
			rankPlayerA, rankPlayerB)
	}

	rankPlayerC := lobby.players[2].Rank
	if rankPlayerC != 2 {
		t.Errorf("new player should be rank 2, since the previous two players had the same rank. (C: %d)", rankPlayerC)
	}
}

func Test_calculateGuesserScore(t *testing.T) {
	lastScore := calculateGuesserScore(0, 0, 115, 120)
	if lastScore >= maxBaseScore {
		t.Errorf("Score should have declined, but was bigger than or "+
			"equal to the baseScore. (LastScore: %d; BaseScore: %d)", lastScore, maxBaseScore)
	}

	lastDecline := -1
	for secondsLeft := 105; secondsLeft >= 5; secondsLeft -= 10 {
		newScore := calculateGuesserScore(0, 0, secondsLeft, 120)
		if newScore > lastScore {
			t.Errorf("Score with more time taken should be lower. (LastScore: %d; NewScore: %d)", lastScore, newScore)
		}
		newDecline := lastScore - newScore
		if lastDecline != -1 && newDecline > lastDecline {
			t.Errorf("Decline should get lower with time taken. (LastDecline: %d; NewDecline: %d)\n", lastDecline, newDecline)
		}
		lastScore = newScore
		lastDecline = newDecline
	}
}

func Test_wordSelectionEvent(t *testing.T) {
	firstWordChoice := "abc"
	lobby := &Lobby{
		mutex: &sync.Mutex{},
		EditableLobbySettings: &EditableLobbySettings{
			DrawingTime: 10,
			Rounds:      10,
		},
		words: []string{firstWordChoice, "def", "ghi"},
	}
	wordHintEvents := make(map[string]*GameEvent)
	lobby.WriteJSON = func(player *SocketConnection, object interface{}) error {
		_, ok := object.(*GameEvent)
		if !ok {
			panic("Unsupported event data type")
		}

		return nil
	}
	drawer := lobby.JoinPlayer(&auth.User{Id: "1234", Name: "Drawer"})
	drawer.Connected = true
	lobby.Owner = drawer
	lobby.creator = drawer

	startError := lobby.HandleEvent(nil, &GameEvent{
		Type: "start",
	}, drawer)
	if startError != nil {
		t.Errorf("Couldn't start lobby: %s", startError)
	}

	guesser := lobby.JoinPlayer(&auth.User{Id: "1235", Name: "Guesser"})
	guesser.Connected = true

	choiceError := lobby.HandleEvent(nil, &GameEvent{
		Type: "choose-word",
		Data: 0,
	}, drawer)
	if choiceError != nil {
		t.Errorf("Couldn't choose word: %s", choiceError)
	}

	wordHintsForDrawerEvent := wordHintEvents[drawer.ID]
	wordHintsForDrawer := wordHintsForDrawerEvent.Data.([]*WordHint)
	if len(wordHintsForDrawer) != 3 {
		t.Errorf("Word hints for drawer were of incorrect length; %d != %d", len(wordHintsForDrawer), 3)
	}

	for index, wordHint := range wordHintsForDrawer {
		if wordHint.Character == 0 {
			t.Error("Word hints for drawer contained invisible character")
		}

		if !wordHint.Underline {
			t.Error("Word hints for drawer contained not underlined character")
		}

		expectedRune := rune(firstWordChoice[index])
		if wordHint.Character != expectedRune {
			t.Errorf("Character at index %d was %c instead of %c", index, wordHint.Character, expectedRune)
		}
	}

	wordHintsForGuesserEvent := wordHintEvents[guesser.ID]
	wordHintsForGuesser := wordHintsForGuesserEvent.Data.([]*WordHint)
	if len(wordHintsForGuesser) != 3 {
		t.Errorf("Word hints for guesser were of incorrect length; %d != %d", len(wordHintsForGuesser), 3)
	}

	for _, wordHint := range wordHintsForGuesser {
		if wordHint.Character != 0 {
			t.Error("Word hints for guesser contained visible character")
		}

		if !wordHint.Underline {
			t.Error("Word hints for guesser contained not underlined character")
		}
	}
}

func Test_kickDrawer(t *testing.T) {
	lobby := &Lobby{
		mutex: &sync.Mutex{},
		EditableLobbySettings: &EditableLobbySettings{
			DrawingTime: 10,
			Rounds:      10,
		},
		words: []string{"a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a"},
	}
	//Dummy to avoid crashes
	lobby.WriteJSON = func(conn *SocketConnection, object interface{}) error {
		return nil
	}

	a := lobby.JoinPlayer(&auth.User{Id: "1234", Name: "TwitchNameA"})
	a.Connected = true
	lobby.Owner = a
	lobby.creator = a

	b := lobby.JoinPlayer(&auth.User{Id: "1235", Name: "TwitchNameB"})
	b.Connected = true
	c := lobby.JoinPlayer(&auth.User{Id: "1236", Name: "TwitchNameC"})
	c.Connected = true

	startError := lobby.HandleEvent(nil, &GameEvent{
		Type: "start",
	}, a)
	if startError != nil {
		t.Errorf("Couldn't start lobby: %s", startError)
	}

	if lobby.drawer == nil {
		t.Error("Drawer should've been a, but was nil")
	}

	if lobby.drawer != a {
		t.Errorf("Drawer should've been a, but was %s", lobby.drawer.Name)
	}

	lobby.Synchronized(func() {
		advanceLobby(lobby)
	})

	if lobby.drawer == nil {
		t.Error("Drawer should've been b, but was nil")
	}

	if lobby.drawer != b {
		t.Errorf("Drawer should've been b, but was %s", lobby.drawer.Name)
	}

	lobby.Synchronized(func() {
		kickPlayer(lobby, b, 1)
	})

	if lobby.drawer == nil {
		t.Error("Drawer should've been c, but was nil")
	}

	if lobby.drawer != c {
		t.Errorf("Drawer should've been c, but was %s", lobby.drawer.Name)
	}
}
