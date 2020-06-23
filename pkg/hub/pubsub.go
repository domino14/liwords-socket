package sockets

import (
	"strings"

	nats "github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// PubSub encapsulates the various subscriptions to the different channels.
// The `liwords` package should have a very similar structure.
type PubSub struct {
	natsconn      *nats.Conn
	topics        []string
	subscriptions []*nats.Subscription
	subchans      map[string]chan *nats.Msg
}

func newPubSub(natsURL string) (*PubSub, error) {
	natsconn, err := nats.Connect(natsURL)

	if err != nil {
		return nil, err
	}

	topics := []string{
		// lobby messages:
		"lobby.>",
		// user messages
		"user.>",
		// usertv messages; for when someone is watching a user's games
		"usertv.>",
		// gametv messages: for observer mode in a single game.
		"gametv.>",
	}
	pubSub := &PubSub{
		natsconn:      natsconn,
		topics:        topics,
		subscriptions: []*nats.Subscription{},
		subchans:      map[string]chan *nats.Msg{},
	}
	// Subscribe to the above topics.
	for _, topic := range topics {
		ch := make(chan *nats.Msg, 64)
		sub, err := natsconn.ChanSubscribe(topic, ch)
		if err != nil {
			return nil, err
		}
		pubSub.subscriptions = append(pubSub.subscriptions, sub)
		pubSub.subchans[topic] = ch

	}
	return pubSub, nil
}

// PubsubProcess processes pubsub messages.
func (h *Hub) PubsubProcess() {
	for {
		select {
		case msg := <-h.pubsub.subchans["lobby.>"]:
			// Handle lobby message. If something is published to the lobby,
			// let's just send it along to the correct sockets, we should not
			// need to parse it.
			log.Debug().Str("topic", msg.Subject).Msg("got lobby message, forwarding along")
			subtopics := strings.Split(msg.Subject, ".")
			if len(subtopics) < 2 {
				log.Error().Msgf("subtopics weird %v", msg.Subject)
				continue
			}
			h.sendToRealm(LobbyRealm, msg.Data)

		case msg := <-h.pubsub.subchans["user.>"]:
			// If we get a user message, we should send it along to the given
			// user.
			log.Debug().Str("topic", msg.Subject).Msg("got user message, forwarding along")
			subtopics := strings.Split(msg.Subject, ".")
			if len(subtopics) < 2 {
				log.Error().Msgf("user subtopics weird %v", msg.Subject)
				continue
			}
			userID := subtopics[1]
			// Send the message to every socket belonging to this user.
			for client := range h.clientsByUserID[userID] {
				client.send <- msg.Data
			}

		case msg := <-h.pubsub.subchans["usertv.>"]:
			// A usertv message is meant for people who are watching a user's games.
			// Find the appropriate Realm.
			log.Debug().Str("topic", msg.Subject).Msg("got usertv message, forwarding along")
			subtopics := strings.Split(msg.Subject, ".")
			if len(subtopics) < 2 {
				log.Error().Msgf("usertv subtopics weird %v", msg.Subject)
				continue
			}
			userID := subtopics[1]
			h.sendToRealm(Realm("usertv-"+userID), msg.Data)

		case msg := <-h.pubsub.subchans["gametv.>"]:
			// A gametv message is meant for people who are observing a user's games.
			log.Debug().Str("topic", msg.Subject).Msg("got gametv message, forwarding along")
			subtopics := strings.Split(msg.Subject, ".")
			if len(subtopics) < 2 {
				log.Error().Msgf("gametv subtopics weird %v", msg.Subject)
				continue
			}
			gameID := subtopics[1]
			h.sendToRealm(Realm("gametv-"+gameID), msg.Data)
		}
	}
}