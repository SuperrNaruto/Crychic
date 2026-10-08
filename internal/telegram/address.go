package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const chatIDBits = 64

// destination names a chat and, for a forum, its topic. Ordinary chats and
// configured channel usernames retain their existing address format.
type destination struct {
	chat   any
	thread int
}

func (d destination) address() string {
	address := fmt.Sprint(d.chat)
	if d.thread > 0 {
		address += "/" + strconv.Itoa(d.thread)
	}
	return address
}

func parseAddress(address string) (destination, error) {
	chat, thread, hasThread := strings.Cut(address, "/")
	if !hasThread && strings.HasPrefix(chat, "@") && len(chat) > 1 {
		return destination{chat: chat}, nil
	}
	id, err := strconv.ParseInt(chat, decimal, chatIDBits)
	if err != nil || id == 0 {
		return destination{}, fmt.Errorf("invalid Telegram address %q", address)
	}
	to := destination{chat: id}
	if hasThread {
		to.thread, err = strconv.Atoi(thread)
		if err != nil || to.thread <= 0 {
			return destination{}, fmt.Errorf("invalid Telegram topic address %q", address)
		}
	}
	return to, nil
}

// Only forum topics carry a destination thread. General-topic replies can
// have a generic message_thread_id, which is not a forum topic selector.
func destinationOf(msg *models.Message) destination {
	to := destination{chat: msg.Chat.ID}
	if msg.IsTopicMessage && msg.MessageThreadID > 0 {
		to.thread = msg.MessageThreadID
	}
	return to
}

func callbackDestination(cq *models.CallbackQuery) destination {
	if msg := cq.Message.Message; msg != nil {
		return destinationOf(msg)
	}
	return destination{chat: cq.From.ID}
}

func (t editTarget) destination() destination {
	return destination{chat: t.chat, thread: t.thread}
}

func inputOf(msg *models.Message) inputKey {
	return inputKey{chat: msg.Chat.ID, user: msg.From.ID, thread: destinationOf(msg).thread}
}

// actorOf keeps the topic in the opaque persisted Address, so arrivals
// remain in their requesting topic even after a restart.
func actorOf(u models.User, to destination) flow.Actor {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.Username
	}
	return flow.Actor{UserID: u.ID, Name: name, Address: to.address()}
}
