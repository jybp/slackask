package slack

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/slack-go/slack"
)

type API struct {
	Client *slack.Client // slack.New(token, slack.OptionHTTPClient(httpCLient))
	Limit  int

	lastMentionTs string // could be in a state outside of mem for restarts
}

type Mention struct {
	Channel   string
	Timestamp string
	Text      string
}

// Should use https://api.slack.com/scopes/app_mentions:read https://api.slack.com/events/app_mention
// But requires a callback URL.
// Leverage api + lastMentionTs for now.
//
// Mentions returns a list of new mentions.
func (api *API) Mentions(ctx context.Context, channelIDs []string, user string) ([]Mention, error) {
	// TODO should handle concurrent access to not return the same mentions.
	mentions := []Mention{}
	for _, channelID := range channelIDs {
		resp, err := api.Client.GetConversationHistoryContext(ctx, &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Limit:     api.Limit,
		})
		if err != nil {
			return nil, fmt.Errorf("GetConversationHistoryContext(,%s): %w", channelID, err)
		}

		// Sort messages by timestamp (oldest first)
		messages := resp.Messages
		sort.Slice(messages, func(i, j int) bool {
			tsI, _ := strconv.ParseFloat(messages[i].Timestamp, 64)
			tsJ, _ := strconv.ParseFloat(messages[j].Timestamp, 64)
			return tsI < tsJ
		})

		for _, msg := range messages {
			if api.lastMentionTs == "" {
				api.lastMentionTs = "0"
			}
			lastTs, err := strconv.ParseFloat(api.lastMentionTs, 64)
			if err != nil {
				return nil, fmt.Errorf("parse last mention timestamp: %w", err)
			}
			ts, err := strconv.ParseFloat(msg.Timestamp, 64)
			if err != nil {
				return nil, fmt.Errorf("parse timestamp: %w", err)
			}
			if ts <= lastTs {
				continue
			}
			// U0LAN0Z89 -> <@U0LAN0Z89>
			botMention := fmt.Sprintf("<@%s>", user)
			if msg.Text == "" || msg.Text == botMention || !strings.Contains(msg.Text, botMention) {
				log.Printf("skipping mention: %s %s", msg.Timestamp, msg.Text)
				continue
			}
			log.Printf("adding mention %s: %s", msg.Timestamp, msg.Text)
			mentions = append(mentions, Mention{
				Channel:   channelID,
				Timestamp: msg.Timestamp,
				Text:      msg.Text,
			})
			if ts > lastTs {
				api.lastMentionTs = msg.Timestamp
			}
		}
	}
	// sort accross channels
	sort.Slice(mentions, func(i, j int) bool {
		its, _ := strconv.ParseFloat(mentions[i].Timestamp, 64)
		jts, _ := strconv.ParseFloat(mentions[j].Timestamp, 64)
		return its < jts
	})
	return mentions, nil
}

func (api API) Reply(ctx context.Context, mention Mention, text string) error {
	_, _, err := api.Client.PostMessageContext(
		ctx,
		mention.Channel,
		slack.MsgOptionPostMessageParameters(slack.PostMessageParameters{
			ThreadTimestamp: mention.Timestamp,
		}),
		slack.MsgOptionText(text, false),
	)
	fmt.Printf("PostMessageContext: %s %s\n", mention.Channel, mention.Timestamp)
	if err != nil {
		return fmt.Errorf("PostMessageContext: %w", err)
	}
	return nil
}
