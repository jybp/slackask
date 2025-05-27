package slack

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/slack-go/slack"
)

type StoreTS interface {
	Get() (string, error)
	Set(string) error
}

type API struct {
	Client *slack.Client // slack.New(token, slack.OptionHTTPClient(httpCLient))
	Limit  int

	StoreTS StoreTS // for storing the last mention timestamp
}

type Mention struct {
	Channel   string
	Timestamp string
	Text      string
}

type FileStoreTS struct {
	path string
}

func NewFileStoreTS() *FileStoreTS {
	return &FileStoreTS{
		path: "/tmp/slackask_last_mention_ts",
	}
}

func (s *FileStoreTS) Get() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func (s *FileStoreTS) Set(ts string) error {
	return os.WriteFile(s.path, []byte(ts), 0644)
}

// Should use https://api.slack.com/scopes/app_mentions:read https://api.slack.com/events/app_mention
// But requires a callback URL.
// Leverage api + lastMentionTs for now.
//
// Mentions returns a list of new mentions.
func (api *API) Mentions(ctx context.Context, channelIDs []string, user string) ([]Mention, error) {
	lastMentionTs, err := api.StoreTS.Get()
	if err != nil {
		return nil, fmt.Errorf("get last mention timestamp: %w", err)
	}
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
				lastMentionTs = msg.Timestamp
			}
		}
	}
	if err := api.StoreTS.Set(lastMentionTs); err != nil {
		return nil, fmt.Errorf("set last mention timestamp: %w", err)
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
