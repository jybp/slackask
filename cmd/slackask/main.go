package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"slackask/gemini"
	"slackask/slack"

	_ "github.com/joho/godotenv/autoload"
	"github.com/jybp/httpthrottle"
	slackgo "github.com/slack-go/slack"
	"golang.org/x/time/rate"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	httpClient := &http.Client{
		Transport: httpthrottle.Default(
			// https://ai.google.dev/gemini-api/docs/rate-limits?authuser=1#free-tier
			rate.NewLimiter(rate.Limit(15/60.0), 1), // 15 RPM
		),
	}

	slackAPI := slack.API{
		Client:  slackgo.New(os.Getenv("SLACK_API_TOKEN"), slackgo.OptionHTTPClient(httpClient)),
		Limit:   100,
		StoreTS: FileStoreTS{Path: "/tmp/slackask_last_mention_ts"},
	}
	geminiAPI := gemini.API{
		Client:  httpClient,
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
		ApiKey:  os.Getenv("GEMINI_API_KEY"),
	}

	mentions, err := slackAPI.Mentions(ctx,
		strings.Split(os.Getenv("SLACH_CHANNELS_IDS"), ","),
		os.Getenv("SLACK_BOT_USER_ID"))
	if err != nil {
		return fmt.Errorf("mentions: %w", err)
	}
	for _, mention := range mentions {
		const prompt = `
You are a helpful slack assistant replying to a user's message.
Your reply should be short, concise and to the point and never refer to the user.
The output should only contain the reply to be sent to slack and be in the slack markdown format.
The user's message you must respond to is:
%s`
		log.Printf("prompt: %s", fmt.Sprintf(prompt, mention.Text))
		response, err := geminiAPI.Prompt(ctx, fmt.Sprintf(prompt, mention.Text))
		if err != nil {
			return fmt.Errorf("prompt: %w", err)
		}
		log.Printf("response: %s", response)
		if err := slackAPI.Reply(ctx, mention, response); err != nil && err.Error() != "cannot_reply_to_message" {
			return fmt.Errorf("reply: %w", err)
		}
	}
	return nil
}

type FileStoreTS struct {
	Path string
}

func (s FileStoreTS) Get() (string, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func (s FileStoreTS) Set(ts string) error {
	return os.WriteFile(s.Path, []byte(ts), 0644)
}
