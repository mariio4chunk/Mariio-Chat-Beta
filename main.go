package main

import (
        "bytes"
        "context"
        "encoding/base64"
        "encoding/json"
        "fmt"
        "io"
        "log"
        "net/http"
        "os"
        "path/filepath"
        "strings"
        "time"

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
        Response    string `json:"response"`
        ImageBase64 string `json:"imageBase64,omitempty"`
        Error       string `json:"error,omitempty"`
}

func main() {
        // Get API keys from environment variables
        // To set this up in Replit Secrets:
        // 1. Go to your Replit project
        // 2. Click on "Secrets" in the left sidebar
        // 3. Add GEMINI_API_KEY from: https://aistudio.google.com/
        // 4. Add HUGGINGFACE_API_KEY from: https://huggingface.co/settings/tokens
        geminiAPIKey := os.Getenv("GEMINI_API_KEY")
        huggingFaceAPIKey := os.Getenv("HUGGINGFACE_API_KEY")
        
        if geminiAPIKey == "" {
                log.Fatal("GEMINI_API_KEY environment variable is required. Please set it in Replit Secrets.")
        }
        if huggingFaceAPIKey == "" {
                log.Fatal("HUGGINGFACE_API_KEY environment variable is required. Please set it in Replit Secrets.")
        }
        
        // Log API key status (safely)
        log.Printf("GEMINI_API_KEY loaded: %t (length: %d)", geminiAPIKey != "", len(geminiAPIKey))
        log.Printf("HUGGINGFACE_API_KEY loaded: %t (length: %d)", huggingFaceAPIKey != "", len(huggingFaceAPIKey))

        // Serve static files from public directory
        fs := http.FileServer(http.Dir("./public/"))
        http.Handle("/", fs)

        // Handle chat API endpoint
        http.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
                handleChat(w, r, geminiAPIKey, huggingFaceAPIKey)
        })

        // Get port from environment or default to 5000
        port := os.Getenv("PORT")
        if port == "" {
                port = "5000"
        }

        fmt.Printf("Server starting on port %s...\n", port)
        fmt.Println("Make sure to set GEMINI_API_KEY and HUGGINGFACE_API_KEY in your Replit Secrets!")
        fmt.Println("Gemini: Text chat and image analysis")
        fmt.Println("Hugging Face: Image generation with Stable Diffusion")
        log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
}

func handleChat(w http.ResponseWriter, r *http.Request, geminiAPIKey, huggingFaceAPIKey string) {
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

        // Check if user wants to generate an image
        isImageGeneration := detectImageGenerationRequest(prompt, messages)
        
        ctx := context.Background()
        var response string
        var imageBase64 string

        if isImageGeneration {
                // Generate image using Hugging Face
                imageBase64, err = generateImage(ctx, huggingFaceAPIKey, prompt)
                if err != nil {
                        log.Printf("Failed to generate image: %v", err)
                        sendErrorResponse(w, "Failed to generate image: "+err.Error(), http.StatusInternalServerError)
                        return
                }
                response = "Saya telah membuat gambar sesuai permintaan Anda!"
        } else {
                // Initialize Gemini client for text/image analysis
                client, err := genai.NewClient(ctx, option.WithAPIKey(geminiAPIKey))
                if err != nil {
                        log.Printf("Failed to initialize Gemini client: %v", err)
                        sendErrorResponse(w, "Failed to initialize Gemini client: "+err.Error(), http.StatusInternalServerError)
                        return
                }
                defer client.Close()

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
        }

        // Log successful response (truncated for readability)
        responsePreview := response
        if len(response) > 100 {
                responsePreview = response[:100] + "..."
        }
        log.Printf("Successfully got response: %s", responsePreview)

        // Send successful response
        chatResponse := ChatResponse{
                Response:    response,
                ImageBase64: imageBase64,
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(chatResponse)
}

func handleTextChat(ctx context.Context, client *genai.Client, messages []Message, prompt string) (string, error) {
        // Use gemini-1.5-flash model for text chat
        model := client.GenerativeModel("gemini-1.5-flash")

        // Prepare the prompt to send
        var promptText string
        if prompt != "" {
                promptText = prompt
        } else if len(messages) > 0 {
                // Use the last message's text
                lastMsg := messages[len(messages)-1]
                for _, part := range lastMsg.Parts {
                        if part.Text != "" {
                                promptText = part.Text
                                break
                        }
                }
        }

        if promptText == "" {
                return "", fmt.Errorf("no text content to send to Gemini")
        }

        // Generate content directly
        resp, err := model.GenerateContent(ctx, genai.Text(promptText))
        if err != nil {
                return "", fmt.Errorf("failed to generate content: %v", err)
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
        // Use gemini-1.5-flash model for image analysis
        model := client.GenerativeModel("gemini-1.5-flash")

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

// detectImageGenerationRequest checks if the user wants to generate an image
func detectImageGenerationRequest(prompt string, messages []Message) bool {
        // Check current prompt
        prompt = strings.ToLower(prompt)
        imageKeywords := []string{
                "buat gambar", "buatkan gambar", "generate image", "create image",
                "draw", "gambar", "lukis", "ilustrasi", "sketch", "photo",
                "picture", "image of", "make a picture", "make an image",
        }
        
        for _, keyword := range imageKeywords {
                if strings.Contains(prompt, keyword) {
                        return true
                }
        }
        
        // Check recent messages for context
        if len(messages) > 0 {
                lastMessage := messages[len(messages)-1]
                for _, part := range lastMessage.Parts {
                        text := strings.ToLower(part.Text)
                        for _, keyword := range imageKeywords {
                                if strings.Contains(text, keyword) {
                                        return true
                                }
                        }
                }
        }
        
        return false
}

// checkModelAvailability checks if a model is available and ready
func checkModelAvailability(ctx context.Context, apiKey, model string) bool {
        url := fmt.Sprintf("https://api-inference.huggingface.co/models/%s", model)
        
        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
                return false
        }
        
        req.Header.Set("Authorization", "Bearer "+apiKey)
        
        client := &http.Client{Timeout: 10 * time.Second}
        resp, err := client.Do(req)
        if err != nil {
                return false
        }
        defer resp.Body.Close()
        
        log.Printf("Model %s availability check: status %d", model, resp.StatusCode)
        return resp.StatusCode == 200
}

// generateImage generates an image using Hugging Face Stable Diffusion API
func generateImage(ctx context.Context, apiKey, prompt string) (string, error) {
        // Try models that are more likely to be available
        models := []string{
                "black-forest-labs/FLUX.1-dev",
                "black-forest-labs/FLUX.1-schnell",
                "stabilityai/stable-diffusion-xl-base-1.0",
                "stabilityai/sdxl-turbo",
                "Lykon/DreamShaper",
                "prompthero/openjourney",
                "nitrosocke/Arcane-Diffusion",
                "runwayml/stable-diffusion-v1-5",
                "CompVis/stable-diffusion-v1-4",
                "stabilityai/stable-diffusion-2-1",
        }
        
        log.Printf("Starting image generation with prompt: %s", prompt)
        
        var lastError error
        var workingModel string
        
        // First, find a working model
        for _, model := range models {
                if checkModelAvailability(ctx, apiKey, model) {
                        workingModel = model
                        log.Printf("Found working model: %s", model)
                        break
                }
        }
        
        if workingModel == "" {
                // If no model responds to availability check, try them anyway
                log.Printf("No model responded to availability check, trying all models anyway")
                workingModel = models[0]
        }
        
        // Try to generate with the working model first, then fallback to others
        modelsToTry := []string{workingModel}
        for _, model := range models {
                if model != workingModel {
                        modelsToTry = append(modelsToTry, model)
                }
        }
        
        for _, model := range modelsToTry {
                log.Printf("Trying image generation with model: %s", model)
                url := fmt.Sprintf("https://api-inference.huggingface.co/models/%s", model)
                
                // Prepare request payload - simplified for better compatibility
                payload := map[string]interface{}{
                        "inputs": prompt,
                }
                
                // Add parameters only for stable diffusion models
                if strings.Contains(model, "stable-diffusion") || strings.Contains(model, "sdxl") {
                        payload["parameters"] = map[string]interface{}{
                                "num_inference_steps": 25,
                                "guidance_scale":      7.5,
                        }
                }
                
                jsonPayload, err := json.Marshal(payload)
                if err != nil {
                        lastError = fmt.Errorf("failed to marshal payload: %v", err)
                        continue
                }
                
                log.Printf("Sending request to %s with payload: %s", url, string(jsonPayload))
                
                // Create HTTP request
                req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
                if err != nil {
                        lastError = fmt.Errorf("failed to create request: %v", err)
                        continue
                }
                
                req.Header.Set("Authorization", "Bearer "+apiKey)
                req.Header.Set("Content-Type", "application/json")
                req.Header.Set("User-Agent", "GeminiChatApp/1.0")
                
                // Send request with longer timeout for image generation
                client := &http.Client{
                        Timeout: 60 * time.Second,
                }
                
                resp, err := client.Do(req)
                if err != nil {
                        lastError = fmt.Errorf("failed to send request: %v", err)
                        log.Printf("Request failed for model %s: %v", model, err)
                        continue
                }
                defer resp.Body.Close()
                
                // Read response body
                body, err := io.ReadAll(resp.Body)
                if err != nil {
                        lastError = fmt.Errorf("failed to read response body: %v", err)
                        continue
                }
                
                log.Printf("Model %s response: status=%d, body_length=%d", model, resp.StatusCode, len(body))
                
                // Log first 200 characters of response for debugging
                if len(body) > 0 {
                        preview := string(body)
                        if len(preview) > 200 {
                                preview = preview[:200] + "..."
                        }
                        log.Printf("Response preview: %s", preview)
                }
                
                if resp.StatusCode == 503 {
                        log.Printf("Model %s is loading (503), will retry in 20 seconds", model)
                        time.Sleep(20 * time.Second)
                        
                        // Retry once
                        resp2, err2 := client.Do(req)
                        if err2 != nil {
                                lastError = fmt.Errorf("retry failed: %v", err2)
                                continue
                        }
                        defer resp2.Body.Close()
                        
                        body, err = io.ReadAll(resp2.Body)
                        if err != nil {
                                lastError = fmt.Errorf("failed to read retry response: %v", err)
                                continue
                        }
                        
                        resp = resp2
                        log.Printf("Retry for model %s: status=%d, body_length=%d", model, resp.StatusCode, len(body))
                } else if resp.StatusCode == 404 {
                        log.Printf("Model %s not found (404), trying next model", model)
                        lastError = fmt.Errorf("model %s not found", model)
                        continue
                } else if resp.StatusCode == 401 {
                        log.Printf("Unauthorized (401) - check your Hugging Face API key")
                        lastError = fmt.Errorf("unauthorized - invalid API key")
                        continue
                } else if resp.StatusCode == 429 {
                        log.Printf("Rate limit exceeded (429), waiting and trying next model")
                        lastError = fmt.Errorf("rate limit exceeded")
                        continue
                }
                
                if resp.StatusCode != http.StatusOK {
                        log.Printf("API request failed for model %s with status %d: %s", model, resp.StatusCode, string(body))
                        lastError = fmt.Errorf("API request failed for model %s with status %d: %s", model, resp.StatusCode, string(body))
                        continue
                }
                
                // Check if response is JSON error
                var errorResp map[string]interface{}
                if json.Unmarshal(body, &errorResp) == nil {
                        if errorMsg, exists := errorResp["error"]; exists {
                                log.Printf("Model %s returned error: %v", model, errorMsg)
                                lastError = fmt.Errorf("model %s returned error: %v", model, errorMsg)
                                continue
                        }
                }
                
                // Check if body is actually image data (binary)
                if len(body) < 100 {
                        log.Printf("Response too short to be an image: %d bytes", len(body))
                        lastError = fmt.Errorf("response too short for model %s", model)
                        continue
                }
                
                // Success! Convert to base64
                imageBase64 := base64.StdEncoding.EncodeToString(body)
                log.Printf("Successfully generated image using model: %s (image size: %d bytes)", model, len(body))
                return imageBase64, nil
        }
        
        // All models failed
        return "", fmt.Errorf("all image generation models failed, last error: %v", lastError)
}

func sendErrorResponse(w http.ResponseWriter, errorMsg string, statusCode int) {
        w.WriteHeader(statusCode)
        response := ChatResponse{
                Error: errorMsg,
        }
        json.NewEncoder(w).Encode(response)
}