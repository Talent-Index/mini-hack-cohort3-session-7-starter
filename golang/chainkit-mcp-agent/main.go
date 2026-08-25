// ChainKit as an MCP server, wired into an agent.
//
// Before running this, start the ChainKit MCP server in another
// terminal:
//
//	npx -y @avalanche-sdk/chainkit mcp-server
//
// It will print the local URL it is running on, e.g.
// http://localhost:PORT/mcp. Put that URL in CHAINKIT_MCP_URL in your
// .env.
//
// This agent then asks a plain-English question, the model decides to
// call a ChainKit tool, and the tool call is forwarded straight to the
// running MCP server, no manual SDK calls for the actual data lookup in
// this file at all.
//
// This file talks to Anthropic directly rather than through the
// modelprovider package, because tool-call round trips need the
// provider's own native message shape (anthropic.MessageParam) threaded
// through the loop, and modelprovider's Message type only covers plain
// text turns today. See modelprovider's package doc for why.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const systemPrompt = "You are Mini Hack Assistant. Use tools when they genuinely help; otherwise answer directly."

func mcpToolsToAnthropic(tools []mcp.Tool) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := anthropic.ToolInputSchemaParam{Properties: t.InputSchema.Properties}
		out = append(out, anthropic.ToolUnionParamOfTool(schema, t.Name))
	}
	return out
}

func main() {
	_ = godotenv.Load()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is not set.")
	}
	mcpURL := os.Getenv("CHAINKIT_MCP_URL")
	if mcpURL == "" {
		log.Fatal("Set CHAINKIT_MCP_URL in your .env first, from the running mcp-server output.")
	}
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	anthropicClient := anthropic.NewClient(anthropicoption.WithAPIKey(apiKey))

	mcpClient, err := client.NewStreamableHttpClient(mcpURL)
	if err != nil {
		log.Fatal("MCP connection error:", err)
	}
	defer mcpClient.Close()

	ctx := context.Background()
	if err := mcpClient.Start(ctx); err != nil {
		log.Fatal("MCP connection error:", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "mini-hack-agent", Version: "1.0.0"}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		log.Fatal("MCP initialize error:", err)
	}

	toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatal("MCP list tools error:", err)
	}
	tools := mcpToolsToAnthropic(toolsResult.Tools)

	fmt.Printf("Mini Hack on-chain agent using anthropic, connected to ChainKit MCP. Type 'exit' to quit.\n\n")

	reader := bufio.NewReader(os.Stdin)
	var messages []anthropic.MessageParam

	for {
		fmt.Print("You: ")
		userInput, _ := reader.ReadString('\n')
		userInput = strings.TrimSpace(userInput)
		if strings.EqualFold(userInput, "exit") {
			break
		}

		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(userInput)))

		resp, err := anthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 1024,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  messages,
			Tools:     tools,
		})
		if err != nil {
			log.Fatal("Agent error:", err)
		}

		for resp.StopReason == "tool_use" {
			assistantBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(resp.Content))
			for _, block := range resp.Content {
				switch block.Type {
				case "text":
					assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(block.Text))
				case "tool_use":
					assistantBlocks = append(assistantBlocks, anthropic.NewToolUseBlock(block.ID, json.RawMessage(block.Input), block.Name))
				}
			}
			messages = append(messages, anthropic.NewAssistantMessage(assistantBlocks...))

			resultBlocks := make([]anthropic.ContentBlockParamUnion, 0)
			for _, block := range resp.Content {
				if block.Type != "tool_use" {
					continue
				}
				var input map[string]any
				_ = json.Unmarshal(block.Input, &input)

				callReq := mcp.CallToolRequest{}
				callReq.Params.Name = block.Name
				callReq.Params.Arguments = input

				result, callErr := mcpClient.CallTool(ctx, callReq)
				if callErr != nil {
					resultBlocks = append(resultBlocks, anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %s", callErr), true))
					continue
				}
				resultJSON, _ := json.Marshal(result.Content)
				resultBlocks = append(resultBlocks, anthropic.NewToolResultBlock(block.ID, string(resultJSON), false))
			}
			messages = append(messages, anthropic.NewUserMessage(resultBlocks...))

			resp, err = anthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
				Model:     anthropic.Model(model),
				MaxTokens: 1024,
				System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
				Messages:  messages,
				Tools:     tools,
			})
			if err != nil {
				log.Fatal("Agent error:", err)
			}
		}

		var finalText string
		for _, block := range resp.Content {
			if block.Type == "text" {
				finalText = block.Text
			}
		}
		fmt.Printf("\nAssistant: %s\n\n", finalText)

		assistantBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(resp.Content))
		for _, block := range resp.Content {
			if block.Type == "text" {
				assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(block.Text))
			}
		}
		messages = append(messages, anthropic.NewAssistantMessage(assistantBlocks...))
	}
}
