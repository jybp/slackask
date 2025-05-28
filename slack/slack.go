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
	Client        *slack.Client // slack.New(token, slack.OptionHTTPClient(httpCLient))
	MessagesLimit int
	RepliesLimit  int
}

type Mention struct {
	SenderUserID string
	Channel      string
	Timestamp    string
	Text         string
}

// Should use https://api.slack.com/scopes/app_mentions:read https://api.slack.com/events/app_mention
// But requires a callback URL.
// Leverage api + lastMentionTs for now.
//
// Mentions returns a list of new mentions.
func (api *API) Mentions(ctx context.Context, channelIDs []string, user string, lastMentionTs string) ([]Mention, error) {
	botMention := fmt.Sprintf("<@%s>", user)

	// TODO should handle concurrent access to not return the same mentions.
	mentions := []Mention{}
	for _, channelID := range channelIDs {
		resp, err := api.Client.GetConversationHistoryContext(ctx, &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Limit:     api.MessagesLimit,
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
			if msg.Text == "" || msg.Text == botMention || !strings.Contains(msg.Text, botMention) {
				log.Printf("skipping mention: %s %s", msg.Timestamp, msg.Text)
				continue
			}
			msgWithReplies := []slack.Message{msg}
			if msg.ReplyCount > 0 {
				replies, _, _, _ := api.Client.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
					ChannelID: channelID,
					Timestamp: msg.Timestamp,
					Limit:     api.RepliesLimit,
				})
				log.Printf("found %d replies for %s", len(replies), msg.Timestamp)
				msgWithReplies = append(msgWithReplies, replies...)
			}
			for _, msg := range msgWithReplies {
				if msg.Text == "" || msg.Text == botMention || !strings.Contains(msg.Text, botMention) {
					log.Printf("skipping mention: %s %s", msg.Timestamp, msg.Text)
					continue
				}
				if lastMentionTs == "" {
					lastMentionTs = "0"
				}
				lastTs, err := strconv.ParseFloat(lastMentionTs, 64)
				if err != nil {
					return nil, fmt.Errorf("parse last mention timestamp: %w", err)
				}
				ts, err := strconv.ParseFloat(msg.Timestamp, 64)
				if err != nil {
					return nil, fmt.Errorf("parse timestamp: %w", err)
				}
				if ts <= lastTs {
					log.Printf("skipping mention: %s %s", msg.Timestamp, msg.Text)
					continue
				}
				log.Printf("adding mention %s: %s", msg.Timestamp, msg.Text)
				mentions = append(mentions, Mention{
					SenderUserID: msg.User,
					Channel:      channelID,
					Timestamp:    msg.Timestamp,
					Text:         msg.Text,
				})
				if ts > lastTs {
					lastMentionTs = msg.Timestamp
				}
			}
		}
	}
	// sort accross channels
	sort.Slice(mentions, func(i, j int) bool {
		its, _ := strconv.ParseFloat(mentions[i].Timestamp, 64)
		jts, _ := strconv.ParseFloat(mentions[j].Timestamp, 64)
		return its < jts
	})

	log.Printf("found %d mentions to reply to", len(mentions))
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
