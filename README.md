# SlackAsk

SlackAsk is a proof-of-concept Slack bot that uses an LLM to answer questions by querying external APIs. It can perform multiple API calls in sequence to gather information before providing a final response.  
Currently supports Google's Gemini AI, but is extensible to other LLM providers.

## How it works

1. Polls Slack channels for new mentions.
2. When it finds a mention, it:
   - Uses the LLM to figure out what API calls are needed, if any
   - Makes those API calls
   - Uses the API responses to generate a reply
3. The reply is posted back to the Slack thread

The bot uses a `specs.json` file in the `cmd/slackask` directory to know which APIs it can query.

## Usage

```bash
# Run once
go run cmd/slackask/main.go

# Run with automatic restart every 10 seconds
go run cmd/slackask/main.go -restart 10s
```

## Environment Variables

- `SLACK_API_TOKEN`: Your Slack bot token
- `SLACK_BOT_USER_ID`: Your bot's user ID
- `SLACH_CHANNELS_IDS`: Comma-separated list of channel IDs to monitor
- `SLACK_REPLY_TO`: Comma-separated list of user IDs allowed to interact with the bot (optional)
- `SLACK_MESSAGES_LIMIT`: Maximum number of messages to fetch per channel (default: 10)
- `SLACK_REPLIES_LIMIT`: Maximum number of replies to fetch per message (default: 10)
- `GEMINI_API_KEY`: Your Google Gemini API key
- `GEMINI_MODEL_URL`: URL of the Gemini model to use (default: https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent)
- `GEMINI_RATE_LIMIT`: Rate limit for Gemini API calls in requests per second (default: 0.25, which is 15 requests per minute)
- `MAX_STEPS`: Maximum number of API calls the bot can make before providing a response (default: 5)
- `API_TOKEN`: The Bearer token used to query the APIs specified in `specs.json`