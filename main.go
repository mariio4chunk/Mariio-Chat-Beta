package main

import (
        "context"
        "encoding/json"
        "fmt"
        "io"
        "log"
        "net/http"
        "os"
        "path/filepath"
        "strings"

        "github.com/google/generative-ai-go/genai"
        "google.golang.org/api/option"
)

// Message represents a chat message in the conversation
type Message struct {
        Role  string      `json:"role"`
        Parts []genai.Part `json:"parts"`
}

// ChatRequest represents the incoming chat request
type ChatRequest struct {
        Messages []Message `json:"messages"`
}

// ChatResponse represents the response sent back to the client
type ChatResponse struct {
        Response string `json:"response"`
        Error    string `json:"error,omitempty"`
}

func main() {
        // Get Gemini API key from environment variable
        apiKey := os.Getenv("GEMINI_API_KEY")
        if apiKey == "" {
                log.Fatal("GEMINI_API_KEY environment variable is required. Please set it in Replit Secrets.")
        }

        // Serve static files from public directory
        fs := http.FileServer(http.Dir("./public/"))
        http.Handle("/", fs)

        // Handle chat API endpoint
        http.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
                handleChat(w, r, apiKey)
        })

        fmt.Println("Server starting on port 5000...")
        fmt.Println("Make sure to set GEMINI_API_KEY in your Replit Secrets!")
        log.Fatal(http.ListenAndServe("0.0.0.0:5000", nil))
}

func handleChat(w http.ResponseWriter, r *http.Request, apiKey string) {
        // Set CORS headers
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        w.Header().Set("Content-Type", "application/json")

        if r.Method == "OPTIONS" {
                return
        }

        if r.Method != "POST" {
                log.Printf("Invalid method: %s", r.Method)
                http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
                return
        }

        // Parse multipart form
        err := r.ParseMultipartForm(10 << 20) // 10 MB max
        if err != nil {
                log.Printf("Failed to parse form data: %v", err)
                sendErrorResponse(w, "Failed to parse form data: "+err.Error())
                return
        }

        // Get messages JSON from form data
        messagesJSON := r.FormValue("messages")
        if messagesJSON == "" {
                log.Println("Messages field is missing")
                sendErrorResponse(w, "Messages field is required")
                return
        }

        log.Printf("Received messages JSON: %s", messagesJSON)

        // Parse messages
        var chatRequest ChatRequest
        err = json.Unmarshal([]byte(messagesJSON), &chatRequest)
        if err != nil {
                log.Printf("Failed to parse messages JSON: %v", err)
                sendErrorResponse(w, "Failed to parse messages JSON: "+err.Error())
                return
        }

        // Check if there's an uploaded image
        file, fileHeader, err := r.FormFile("image")
        var hasImage bool
        var imageData []byte
        var mimeType string

        if err == nil {
                hasImage = true
                defer file.Close()

                // Read image data
                imageData, err = io.ReadAll(file)
                if err != nil {
                        sendErrorResponse(w, "Failed to read image data: "+err.Error())
                        return
                }

                // Determine MIME type from file extension
                ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
                switch ext {
                case ".jpg", ".jpeg":
                        mimeType = "image/jpeg"
                case ".png":
                        mimeType = "image/png"
                case ".gif":
                        mimeType = "image/gif"
                case ".webp":
                        mimeType = "image/webp"
                default:
                        mimeType = "image/jpeg" // default fallback
                }
        }

        // Initialize Gemini client
        ctx := context.Background()
        client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
        if err != nil {
                log.Printf("Failed to initialize Gemini client: %v", err)
                sendErrorResponse(w, "Failed to initialize Gemini client: "+err.Error())
                return
        }
        defer client.Close()

        var response string

        if hasImage {
                log.Printf("Processing image chat with image size: %d bytes, mimeType: %s", len(imageData), mimeType)
                // Use gemini-pro-vision for image analysis
                response, err = handleImageChat(ctx, client, chatRequest.Messages, imageData, mimeType)
        } else {
                log.Printf("Processing text-only chat with %d messages in history", len(chatRequest.Messages))
                // Use gemini-pro for text-only chat
                response, err = handleTextChat(ctx, client, chatRequest.Messages)
        }

        if err != nil {
                log.Printf("Failed to get response from Gemini: %v", err)
                sendErrorResponse(w, "Failed to get response from Gemini: "+err.Error())
                return
        }

        responsePreview := response
        if len(response) > 100 {
                responsePreview = response[:100]
        }
        log.Printf("Successfully got response from Gemini: %s", responsePreview)

        // Send successful response
        chatResponse := ChatResponse{
                Response: response,
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(chatResponse)
}

func handleTextChat(ctx context.Context, client *genai.Client, messages []Message) (string, error) {
        // Use gemini-pro model for text chat
        model := client.GenerativeModel("gemini-pro")
        
        // Configure model settings
        model.SetTemperature(0.7)
        model.SetTopK(40)
        model.SetTopP(0.95)
        model.SetMaxOutputTokens(2048)

        // Start chat session with history
        chat := model.StartChat()

        // Add conversation history (exclude the last message which we'll send separately)
        for i, msg := range messages {
                if i < len(messages)-1 {
                        chat.History = append(chat.History, &genai.Content{
                                Role:  msg.Role,
                                Parts: msg.Parts,
                        })
                }
        }

        // Send the latest message
        if len(messages) > 0 {
                lastMessage := messages[len(messages)-1]
                resp, err := chat.SendMessage(ctx, lastMessage.Parts...)
                if err != nil {
                        return "", err
                }

                // Extract text from response
                if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
                        if textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
                                return string(textPart), nil
                        }
                }
        }

        return "Sorry, I couldn't generate a response.", nil
}

func handleImageChat(ctx context.Context, client *genai.Client, messages []Message, imageData []byte, mimeType string) (string, error) {
        // Use gemini-pro-vision model for image analysis
        model := client.GenerativeModel("gemini-pro-vision")
        
        // Configure model settings
        model.SetTemperature(0.4)
        model.SetTopK(32)
        model.SetTopP(1)
        model.SetMaxOutputTokens(2048)

        // Get the user's text prompt (if any) from the last message
        var userPrompt string
        if len(messages) > 0 {
                lastMessage := messages[len(messages)-1]
                for _, part := range lastMessage.Parts {
                        if textPart, ok := part.(genai.Text); ok {
                                userPrompt = string(textPart)
                                break
                        }
                }
        }

        // If no text prompt, use a default one
        if userPrompt == "" {
                userPrompt = "Analyze this image and describe what you see."
        }

        // Create image part
        imagePart := genai.ImageData(mimeType, imageData)

        // Generate content with both text and image
        resp, err := model.GenerateContent(ctx, genai.Text(userPrompt), imagePart)
        if err != nil {
                return "", err
        }

        // Extract text from response
        if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
                if textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
                        return string(textPart), nil
                }
        }

        return "Sorry, I couldn't analyze the image.", nil
}

func sendErrorResponse(w http.ResponseWriter, errorMsg string) {
        w.WriteHeader(http.StatusBadRequest)
        response := ChatResponse{
                Error: errorMsg,
        }
        json.NewEncoder(w).Encode(response)
}
