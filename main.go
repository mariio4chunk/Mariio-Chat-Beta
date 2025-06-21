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

// MessagePart represents a part of a message (text or image data)
type MessagePart struct {
        Text     string `json:"text,omitempty"`
        MimeType string `json:"mimeType,omitempty"`
        Data     string `json:"data,omitempty"` // base64-encoded for images
}

// Message represents a chat message in the conversation
type Message struct {
        Role  string        `json:"role"`
        Parts []MessagePart `json:"parts"`
}

// ChatResponse represents the response sent back to the client
type ChatResponse struct {
        Response string `json:"response"`
        Error    string `json:"error,omitempty"`
}

func main() {
        // Get Gemini API key from environment variable
        // To set this up in Replit Secrets:
        // 1. Go to your Replit project
        // 2. Click on "Secrets" in the left sidebar
        // 3. Add a new secret with key: GEMINI_API_KEY
        // 4. Set the value to your Google AI Studio API key
        // 5. Get your API key from: https://aistudio.google.com/
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

        // Get port from environment or default to 5000
        port := os.Getenv("PORT")
        if port == "" {
                port = "5000"
        }

        fmt.Printf("Server starting on port %s...\n", port)
        fmt.Println("Make sure to set GEMINI_API_KEY in your Replit Secrets!")
        log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
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
                sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
                return
        }

        // Parse multipart form with 10MB limit
        err := r.ParseMultipartForm(10 << 20)
        if err != nil {
                log.Printf("Failed to parse multipart form: %v", err)
                sendErrorResponse(w, "Failed to parse form data: "+err.Error(), http.StatusBadRequest)
                return
        }

        // Extract messages JSON from form data
        messagesJSON := r.FormValue("messages")
        if messagesJSON == "" {
                log.Println("Messages field is missing from request")
                sendErrorResponse(w, "Messages field is required", http.StatusBadRequest)
                return
        }

        log.Printf("Received messages JSON: %s", messagesJSON)

        // Parse messages array
        var messages []Message
        err = json.Unmarshal([]byte(messagesJSON), &messages)
        if err != nil {
                log.Printf("Failed to parse messages JSON: %v", err)
                sendErrorResponse(w, "Failed to parse messages JSON: "+err.Error(), http.StatusBadRequest)
                return
        }

        log.Printf("Successfully parsed %d messages", len(messages))

        // Extract prompt text
        prompt := r.FormValue("prompt")
        log.Printf("Received prompt: %s", prompt)

        // Check for uploaded image
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
                        log.Printf("Failed to read image data: %v", err)
                        sendErrorResponse(w, "Failed to read image data: "+err.Error(), http.StatusBadRequest)
                        return
                }

                // Detect MIME type from file extension
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
                        // Try to detect content type
                        mimeType = http.DetectContentType(imageData)
                        if !strings.HasPrefix(mimeType, "image/") {
                                mimeType = "image/jpeg" // fallback
                        }
                }

                log.Printf("Processing image: size=%d bytes, mimeType=%s", len(imageData), mimeType)
        }

        // Initialize Gemini client
        ctx := context.Background()
        client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
        if err != nil {
                log.Printf("Failed to initialize Gemini client: %v", err)
                sendErrorResponse(w, "Failed to initialize Gemini client: "+err.Error(), http.StatusInternalServerError)
                return
        }
        defer client.Close()

        var response string

        if hasImage {
                // Use gemini-pro-vision for image analysis
                response, err = handleImageChat(ctx, client, messages, imageData, mimeType, prompt)
        } else {
                // Use gemini-pro for text-only chat
                response, err = handleTextChat(ctx, client, messages, prompt)
        }

        if err != nil {
                log.Printf("Failed to get response from Gemini: %v", err)
                sendErrorResponse(w, "Failed to get response from Gemini: "+err.Error(), http.StatusInternalServerError)
                return
        }

        // Log successful response (truncated for readability)
        responsePreview := response
        if len(response) > 100 {
                responsePreview = response[:100] + "..."
        }
        log.Printf("Successfully got response from Gemini: %s", responsePreview)

        // Send successful response
        chatResponse := ChatResponse{
                Response: response,
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(chatResponse)
}

func handleTextChat(ctx context.Context, client *genai.Client, messages []Message, prompt string) (string, error) {
        // Use gemini-pro model for text chat
        model := client.GenerativeModel("gemini-pro")

        // Configure model settings
        model.SetTemperature(0.7)
        model.SetTopK(40)
        model.SetTopP(0.95)
        model.SetMaxOutputTokens(2048)

        // Start chat session
        chat := model.StartChat()

        // Build conversation history from messages
        for i, msg := range messages {
                var parts []genai.Part
                
                for _, part := range msg.Parts {
                        if part.Text != "" {
                                parts = append(parts, genai.Text(part.Text))
                        }
                        // For text-only chat, skip image parts from history
                }

                if len(parts) > 0 {
                        // Add to chat history (exclude the last message which we'll send separately)
                        if i < len(messages)-1 {
                                chat.History = append(chat.History, &genai.Content{
                                        Role:  msg.Role,
                                        Parts: parts,
                                })
                        }
                }
        }

        // Determine what to send as the current message
        var currentParts []genai.Part
        
        if prompt != "" {
                currentParts = append(currentParts, genai.Text(prompt))
        } else if len(messages) > 0 {
                // Use the last message's text parts
                lastMsg := messages[len(messages)-1]
                for _, part := range lastMsg.Parts {
                        if part.Text != "" {
                                currentParts = append(currentParts, genai.Text(part.Text))
                        }
                }
        }

        if len(currentParts) == 0 {
                return "", fmt.Errorf("no text content to send to Gemini")
        }

        // Send the current message
        resp, err := chat.SendMessage(ctx, currentParts...)
        if err != nil {
                return "", fmt.Errorf("failed to send message to Gemini: %v", err)
        }

        // Extract text from response
        if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
                if textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
                        return string(textPart), nil
                }
        }

        return "Maaf, saya tidak dapat menghasilkan respons saat ini.", nil
}

func handleImageChat(ctx context.Context, client *genai.Client, messages []Message, imageData []byte, mimeType, prompt string) (string, error) {
        // Use gemini-pro-vision model for image analysis
        model := client.GenerativeModel("gemini-pro-vision")

        // Configure model settings
        model.SetTemperature(0.4)
        model.SetTopK(32)
        model.SetTopP(1)
        model.SetMaxOutputTokens(2048)

        // Prepare parts for the current request
        var parts []genai.Part

        // Add text prompt if provided
        if prompt != "" {
                parts = append(parts, genai.Text(prompt))
        } else {
                // Default prompt for image analysis
                parts = append(parts, genai.Text("Analisis gambar ini dan jelaskan apa yang Anda lihat."))
        }

        // Add the current image
        imagePart := genai.ImageData(mimeType, imageData)
        parts = append(parts, imagePart)

        // Generate content with text and image
        resp, err := model.GenerateContent(ctx, parts...)
        if err != nil {
                return "", fmt.Errorf("failed to generate content with image: %v", err)
        }

        // Extract text from response
        if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
                if textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
                        return string(textPart), nil
                }
        }

        return "Maaf, saya tidak dapat menganalisis gambar ini.", nil
}

func sendErrorResponse(w http.ResponseWriter, errorMsg string, statusCode int) {
        w.WriteHeader(statusCode)
        response := ChatResponse{
                Error: errorMsg,
        }
        json.NewEncoder(w).Encode(response)
}