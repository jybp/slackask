package llmchain

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// LLMClient defines the interface for interacting with a language model
type LLMClient interface {
	// Prompt sends a prompt to the language model and returns its response
	Prompt(ctx context.Context, prompt string) (string, error)
}

// APIClient defines the interface for making API calls
type APIClient interface {
	// Query performs a GET request to the API and returns the response body
	Query(ctx context.Context, queryURL string) ([]byte, error)
}

// Step represents a single step in the chain that can either make an API call or provide a final response
type Step struct {
	// Response is the final response to be sent to the user
	Response string `json:"response,omitempty"`
	// APIQuery is the URL to query if an API call is needed
	APIQuery string `json:"api_query,omitempty"`
}

// Chain represents a chain of LLM interactions with potential API calls
type Chain struct {
	// LLM is the interface for interacting with the language model
	LLM LLMClient
	// API is the interface for making API calls
	API APIClient
	// MaxSteps is the maximum number of steps allowed in the chain
	MaxSteps int
}

// Execute runs the chain of interactions
func (c *Chain) Execute(ctx context.Context, initialPrompt string) (string, error) {
	var contextBuilder strings.Builder
	stepCount := 0

	log.Printf("starting chain execution")

	for {
		if stepCount >= c.MaxSteps {
			return "", fmt.Errorf("max steps exceeded")
		}

		// Build the prompt with accumulated context
		prompt := fmt.Sprintf("%s\n\nContext from previous steps:\n%s", initialPrompt, contextBuilder.String())
		log.Printf("step %d: sending prompt to LLM (context length: %d)", stepCount+1, contextBuilder.Len())

		// Get response from LLM
		response, err := c.LLM.Prompt(ctx, prompt)
		if err != nil {
			log.Printf("LLM prompt failed: %v", err)
			return "", fmt.Errorf("llm prompt: %w", err)
		}
		log.Printf("received LLM response: %s", response)

		// Clean the response by removing markdown code blocks
		response = strings.TrimPrefix(response, "```json\n")
		response = strings.TrimPrefix(response, "```\n")
		response = strings.TrimSuffix(response, "\n```")
		response = strings.TrimSpace(response)
		log.Printf("cleaned response: %s", response)

		// Parse the response
		var step Step
		if err := json.Unmarshal([]byte(response), &step); err != nil {
			log.Printf("failed to parse LLM response: %v (response: %s)", err, response)
			return "", fmt.Errorf("unmarshal response %s: %w", response, err)
		}
		log.Printf("parsed step: %+v", step)

		// If we got a final response, return it
		if step.Response != "" {
			log.Printf("chain completed with final response after %d steps: %s", stepCount+1, step.Response)
			return step.Response, nil
		}

		// If we got an API query, execute it and add to context
		if step.APIQuery != "" {
			log.Printf("executing API query: %s", step.APIQuery)
			body, err := c.API.Query(ctx, step.APIQuery)
			if err != nil {
				log.Printf("API query failed: %v", err)
				return "", fmt.Errorf("api query: %w", err)
			}

			additionalContext := fmt.Sprintf("\nAPI Response from %s:\n%s\n", step.APIQuery, string(body))
			log.Printf("additional context: %s", additionalContext)
			// Add the API response to our context
			contextBuilder.WriteString(additionalContext)
			log.Printf("added API response to context: %s", string(body))
		}

		stepCount++
	}
}
