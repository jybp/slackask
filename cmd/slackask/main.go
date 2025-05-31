package main

import (
	"context"
	_ "embed"
	"flag"
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

// specs.json should be placed in the cmd/slackask directory
// It contains the OpenAPI specification for the APIs the bot can query
//
//go:embed specs.json
var specs string

var (
	restartDuration time.Duration
)

func init() {
	flag.DurationVar(&restartDuration, "restart", 0, "restart duration (e.g. 10s, 1m). If not set, runs once")
	flag.Parse()
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	log.Printf("specs: %s", specs)
	for {
		if err := run(ctx); err != nil {
			log.Printf("error in run: %v", err)
		}
		if restartDuration == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(restartDuration):
		}
	}
}

func run(ctx context.Context) error {

	messagesLimit, repliesLimit := 10, 10
	if value := os.Getenv("SLACK_MESSAGES_LIMIT"); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			messagesLimit = intValue
		}
	}
	if value := os.Getenv("SLACK_REPLIES_LIMIT"); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			repliesLimit = intValue
		}
	}

	slackAPI := slack.API{
		Client:        slackgo.New(os.Getenv("SLACK_API_TOKEN"), slackgo.OptionHTTPClient(http.DefaultClient)),
		MessagesLimit: messagesLimit,
		RepliesLimit:  repliesLimit,
	}

	errorSlack := func(mention slack.Mention, err error) {
		if err != nil && err.Error() != "cannot_reply_to_message" {
			if err := slackAPI.Reply(ctx, mention, "I'm sorry, I'm having trouble responding to your message. Please try again later."); err != nil && err.Error() != "cannot_reply_to_message" {
				log.Printf("error replying to mention: %v", err)
			}
		}
	}

	geminiRateLimit := 15.0 / 60.0 // 15 RPM by default
	if value := os.Getenv("GEMINI_RATE_LIMIT"); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			geminiRateLimit = floatValue
		}
	}

	geminiModelURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
	if value := os.Getenv("GEMINI_MODEL_URL"); value != "" {
		geminiModelURL = value
	}

	llmClient := &http.Client{
		Transport: httpthrottle.Default(
			rate.NewLimiter(rate.Limit(geminiRateLimit), 1),
		),
	}
	geminiAPI := gemini.API{
		Client:  llmClient,
		BaseURL: geminiModelURL,
		ApiKey:  os.Getenv("GEMINI_API_KEY"),
	}
	apiClient := apiclient.Client{
		Client: http.DefaultClient,
		Token:  os.Getenv("API_TOKEN"),
	}

	maxSteps := 5
	if value := os.Getenv("MAX_STEPS"); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			maxSteps = intValue
		}
	}

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

	var allowedUserIDs []string
	if len(os.Getenv("SLACK_REPLY_TO")) > 0 {
		allowedUserIDs = strings.Split(os.Getenv("SLACK_REPLY_TO"), ",")
	}

	for _, mention := range mentions {

		if len(allowedUserIDs) > 0 && !strings.Contains(strings.Join(allowedUserIDs, ","), mention.SenderUserID) {
			log.Printf("skipping mention from user %s: %v", mention.SenderUserID, allowedUserIDs)
			if err := slackAPI.Reply(ctx, mention, "https://media.tenor.com/-7miMPOSr9EAAAAM/who-da-fook-conor-mcgregor.gif"); err != nil && err.Error() != "cannot_reply_to_message" {
				return fmt.Errorf("reply to mention %s with response whoisthatguy: %w", mention, err)
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
