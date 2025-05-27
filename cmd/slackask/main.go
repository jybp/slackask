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
	slackgo "github.com/slack-go/slack"
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

	httpClient := http.DefaultClient

	slackAPI := slack.API{
		Client: slackgo.New(os.Getenv("SLACK_API_TOKEN"), slackgo.OptionHTTPClient(httpClient)),
		Limit:  10,
	}
	geminiAPI := gemini.API{
		Client: httpClient,
		ApiKey: os.Getenv("GEMINI_API_KEY"),
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
		if err := slackAPI.Reply(ctx, mention, response); err != nil {
			return fmt.Errorf("reply: %w", err)
		}
	}
	return nil
}
