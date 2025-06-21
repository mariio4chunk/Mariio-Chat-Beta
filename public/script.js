class GeminiChat {
    constructor() {
        this.messages = [];
        this.isLoading = false;
        this.selectedImageFile = null;
        this.isFirstMessage = true;
        
        this.initializeElements();
        this.attachEventListeners();
        this.updateSendButtonState();
    }

    initializeElements() {
        // Get all DOM element references
        this.welcomeScreen = document.getElementById('welcome-screen');
        this.chatContainer = document.getElementById('chat-container');
        this.userInput = document.getElementById('user-input');
        this.sendButton = document.getElementById('send-button');
        this.imageUpload = document.getElementById('image-upload');
        this.imagePreview = document.getElementById('image-preview');
        this.imagePreviewContainer = document.getElementById('image-preview-container');
        this.clearImagePreview = document.getElementById('clear-image-preview');
        this.loadingIndicator = document.getElementById('loading-indicator');
        this.errorMessageArea = document.getElementById('error-message-area');
    }

    attachEventListeners() {
        // Send button click
        this.sendButton.addEventListener('click', () => this.sendMessage());

        // Enter key press (Shift+Enter for new line)
        this.userInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.sendMessage();
            }
        });

        // Auto-resize textarea and update send button state
        this.userInput.addEventListener('input', () => {
            this.autoResizeTextarea();
            this.updateSendButtonState();
        });

        // Image upload
        this.imageUpload.addEventListener('change', (e) => this.handleImageUpload(e));

        // Clear image preview
        this.clearImagePreview.addEventListener('click', () => this.clearImage());
    }

    autoResizeTextarea() {
        const textarea = this.userInput;
        textarea.style.height = 'auto';
        const scrollHeight = textarea.scrollHeight;
        const maxHeight = 120; // Max height in pixels
        
        if (scrollHeight <= maxHeight) {
            textarea.style.height = `${scrollHeight}px`;
        } else {
            textarea.style.height = `${maxHeight}px`;
        }
    }

    updateSendButtonState() {
        const hasText = this.userInput.value.trim().length > 0;
        const hasImage = this.selectedImageFile !== null;
        
        this.sendButton.disabled = (!hasText && !hasImage) || this.isLoading;
    }

    handleImageUpload(event) {
        const file = event.target.files[0];
        if (!file) return;

        // Validate file type
        if (!file.type.startsWith('image/')) {
            this.showError('Harap pilih file gambar yang valid.');
            return;
        }

        // Validate file size (10MB max)
        if (file.size > 10 * 1024 * 1024) {
            this.showError('Ukuran file gambar harus kurang dari 10MB.');
            return;
        }

        this.selectedImageFile = file;
        this.showImagePreview(file);
        this.updateSendButtonState();
    }

    showImagePreview(file) {
        const reader = new FileReader();
        reader.onload = (e) => {
            this.imagePreview.src = e.target.result;
            this.imagePreviewContainer.classList.add('show');
        };
        reader.readAsDataURL(file);
    }

    clearImage() {
        this.imagePreviewContainer.classList.remove('show');
        setTimeout(() => {
            this.imagePreview.src = '';
        }, 300);
        
        this.selectedImageFile = null;
        this.imageUpload.value = '';
        this.updateSendButtonState();
    }

    hideWelcomeScreen() {
        this.welcomeScreen.classList.add('fade-out');
        setTimeout(() => {
            this.welcomeScreen.style.display = 'none';
        }, 500);
    }

    async sendMessage() {
        const text = this.userInput.value.trim();
        const image = this.selectedImageFile;

        if ((!text && !image) || this.isLoading) return;

        // Hide welcome screen on first message
        if (this.isFirstMessage) {
            this.hideWelcomeScreen();
            this.isFirstMessage = false;
        }

        // Hide error messages
        this.hideError();

        // Create user message
        const userMessage = {
            role: 'user',
            parts: []
        };

        if (text) {
            userMessage.parts.push({ text: text });
        }

        // Add user message to messages array and render
        this.messages.push(userMessage);
        this.renderMessage(userMessage, image);

        // Clear input
        this.userInput.value = '';
        this.clearImage();
        this.autoResizeTextarea();

        // Show loading indicator
        this.showLoadingIndicator();

        try {
            // Prepare FormData
            const formData = new FormData();
            formData.append('messages', JSON.stringify(this.messages));

            if (image) {
                formData.append('image', image);
            }

            // Send to backend
            const response = await fetch('/api/chat', {
                method: 'POST',
                body: formData
            });

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            const data = await response.json();
            
            if (data.error) {
                throw new Error(data.error);
            }

            // Add AI response to messages and render
            const aiMessage = {
                role: 'model',
                parts: [{ text: data.response }]
            };

            this.messages.push(aiMessage);
            this.renderMessage(aiMessage);
            
        } catch (error) {
            console.error('Error sending message:', error);
            this.showError('Gagal mendapatkan respons dari Gemini. Silakan coba lagi.');
        } finally {
            this.hideLoadingIndicator();
        }
    }

    renderMessage(message, imageFile = null) {
        const messageElement = document.createElement('div');
        messageElement.className = `message ${message.role}-message`;

        let content = '';

        // Add image if present (for user messages)
        if (imageFile && message.role === 'user') {
            const imageUrl = URL.createObjectURL(imageFile);
            content += `<img src="${imageUrl}" alt="Uploaded image" class="message-image">`;
        }

        // Add text content
        message.parts.forEach(part => {
            if (part.text) {
                if (message.role === 'model') {
                    content += `<div>${this.formatAIResponse(part.text)}</div>`;
                } else {
                    content += `<div>${this.escapeHtml(part.text)}</div>`;
                }
            }
        });

        messageElement.innerHTML = `<div class="message-content">${content}</div>`;

        this.chatContainer.appendChild(messageElement);
        this.autoScrollChat();
    }

    formatAIResponse(text) {
        // Format AI responses with basic markdown support
        let formatted = this.escapeHtml(text);
        
        // Convert line breaks to HTML
        formatted = formatted.replace(/\n/g, '<br>');
        
        // Make text bold when surrounded by **
        formatted = formatted.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
        
        // Make text italic when surrounded by *
        formatted = formatted.replace(/\*(.*?)\*/g, '<em>$1</em>');
        
        return formatted;
    }

    showLoadingIndicator() {
        this.isLoading = true;
        this.loadingIndicator.classList.add('show');
        this.updateSendButtonState();
        this.autoScrollChat();
    }

    hideLoadingIndicator() {
        this.isLoading = false;
        this.loadingIndicator.classList.remove('show');
        this.updateSendButtonState();
    }

    showError(message) {
        this.errorMessageArea.textContent = message;
        this.errorMessageArea.classList.add('show');
        this.autoScrollChat();

        // Hide error after 5 seconds
        setTimeout(() => {
            this.hideError();
        }, 5000);
    }

    hideError() {
        this.errorMessageArea.classList.remove('show');
        setTimeout(() => {
            this.errorMessageArea.textContent = '';
        }, 300);
    }

    autoScrollChat() {
        setTimeout(() => {
            this.chatContainer.scrollTop = this.chatContainer.scrollHeight;
        }, 100);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Initialize the chat application when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.geminiChat = new GeminiChat();
});