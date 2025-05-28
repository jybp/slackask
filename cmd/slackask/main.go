package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"slackask/apiclient"
	"slackask/gemini"
	"slackask/llmchain"
	"slackask/slack"

	_ "github.com/joho/godotenv/autoload"
	"github.com/jybp/httpthrottle"
	slackgo "github.com/slack-go/slack"
	"golang.org/x/time/rate"
)

func main() {
	for {
		if err := run(); err != nil {
			log.Printf("error in run: %v", err)
		}
		log.Printf("waiting 10 seconds before next run")
		time.Sleep(10 * time.Second)
	}
}

var specs string

func init() {
	specsb, err := os.ReadFile("spec.json")
	if err != nil {
		panic(err)
	}
	specs = string(specsb)
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	slackAPI := slack.API{
		Client:        slackgo.New(os.Getenv("SLACK_API_TOKEN"), slackgo.OptionHTTPClient(http.DefaultClient)),
		MessagesLimit: 10,
		RepliesLimit:  10,
	}

	errorSlack := func(mention slack.Mention, err error) {
		if err != nil && err.Error() != "cannot_reply_to_message" {
			if err := slackAPI.Reply(ctx, mention, "I'm sorry, I'm having trouble responding to your message. Please try again later."); err != nil && err.Error() != "cannot_reply_to_message" {
				log.Printf("error replying to mention: %v", err)
			}
		}
	}

	llmClient := &http.Client{
		Transport: httpthrottle.Default(
			// https://ai.google.dev/gemini-api/docs/rate-limits?authuser=1#free-tier
			rate.NewLimiter(rate.Limit(15/60.0), 1), // 15 RPM
		),
	}
	geminiAPI := gemini.API{
		Client:  llmClient,
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
		ApiKey:  os.Getenv("GEMINI_API_KEY"),
	}

	apiClient := apiclient.Client{
		Client: http.DefaultClient,
		Token:  os.Getenv("KILN_API_TOKEN"),
	}

	const maxSteps = 5
	chain := llmchain.Chain{
		LLM:      &geminiAPI,
		API:      &apiClient,
		MaxSteps: maxSteps,
	}

	storeTS := FileStoreTS{Path: "/tmp/slackask_last_mention_ts"}
	lastMentionTs, err := storeTS.Get()
	if err != nil {
		return fmt.Errorf("get last mention timestamp: %w", err)
	}
	mentions, err := slackAPI.Mentions(ctx,
		strings.Split(os.Getenv("SLACH_CHANNELS_IDS"), ","),
		os.Getenv("SLACK_BOT_USER_ID"),
		lastMentionTs)
	if err != nil {
		return fmt.Errorf("mentions: %w", err)
	}
	if len(mentions) == 0 {
		log.Printf("no mentions")
		return nil
	}

	allowedUserIDs := strings.Split(os.Getenv("SLACK_ALLOWED_USER_IDS"), ",")
	if len(allowedUserIDs) == 0 {
		return fmt.Errorf("no allowed user IDs specified in SLACK_ALLOWED_USER_IDS")
	}

	// mentions = mentions[:1]
	for _, mention := range mentions {

		if !strings.Contains(strings.Join(allowedUserIDs, ","), mention.SenderUserID) {
			log.Printf("skipping mention from user %s: %v", mention.SenderUserID, allowedUserIDs)
			if err := slackAPI.Reply(ctx, mention, "https://media.tenor.com/-7miMPOSr9EAAAAM/who-da-fook-conor-mcgregor.gif"); err != nil && err.Error() != "cannot_reply_to_message" {
				// return fmt.Errorf("reply to mention %s with response whoisthatguy: %w", mention, err)
			}
			if err := storeTS.Set(mention.Timestamp); err != nil {
				return fmt.Errorf("set last mention timestamp: %w", err)
			}
			continue
		}

		var prompt = `
You are a helpful slack assistant replying to a user's message.
Today the date is %s.
You can query external APIs to gather information needed to respond to the user.

Your response must be a JSON object with exactly one of these fields:

- api_query: The full GET URL with parameters to be sent to the API
- response: the text of the unique reply to be sent to slack in the slack markdown format that is short and to the point. You must list at the end all the API URLs calls that were made.

The response must be a valid json.
You are interracting with a program that allows you to perform api queries with an access token. You will have access to the api response.
You must return only api_query alone initially.
You must never reply with a response saying you are about to perform an api query, you must just return the api_query in that case.
Otherwise continue to make api_query steps until you have all the information you need to answer the user's question.
You have a maximum of ` + strconv.Itoa(maxSteps) + ` api_query steps before you must send a response.

The user's message you must respond to is:
%s

The API specs are:
%s`

		response, err := chain.Execute(ctx, fmt.Sprintf(prompt, time.Now().Format("2006-01-02"), mention.Text, specs))
		if err != nil {
			errorSlack(mention, err)
			return fmt.Errorf("chain execute: %w", err)
		}

		if err := slackAPI.Reply(ctx, mention, response); err != nil && err.Error() != "cannot_reply_to_message" {
			return fmt.Errorf("reply to mention %s with response %s: %w", mention, response, err)
		}
		if err := storeTS.Set(mention.Timestamp); err != nil {
			return fmt.Errorf("set last mention timestamp: %w", err)
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
