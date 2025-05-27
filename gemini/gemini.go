package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// https://ai.google.dev/gemini-api/docs/quickstart?lang=python&authuser=1#go

type API struct {
	BaseURL string
	Client  *http.Client
	ApiKey  string
}

type requestBody struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type responseBody struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

//	curl "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=GEMINI_API_KEY" \
//	  -H 'Content-Type: application/json' \
//	  -X POST \
//	  -d '{
//	    "contents": [
//	      {
//	        "parts": [
//	          {
//	            "text": "Explain how AI works in a few words"
//	          }
//	        ]
//	      }
//	    ]
//	  }'
//
// Prompt sends a text prompt to the Gemini API and returns the response
func (api *API) Prompt(ctx context.Context, text string) (string, error) {
	reqBody := requestBody{
		Contents: []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{
			{
				Parts: []struct {
					Text string `json:"text"`
				}{
					{Text: text},
				},
			},
		},
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	url := fmt.Sprintf("%s?key=%s", api.BaseURL, api.ApiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	var respBody responseBody
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(respBody.Candidates) == 0 || len(respBody.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from API")
	}
	return respBody.Candidates[0].Content.Parts[0].Text, nil
}
