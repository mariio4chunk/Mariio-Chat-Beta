class GeminiChat {
    constructor() {
        this.messages = [];
        this.selectedImageFile = null;
        this.isLoading = false;
        
        this.initializeElements();
        this.attachEventListeners();
        this.updateSendButtonState();
    }

    initializeElements() {
        // Get DOM element references
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

        // Auto-resize textarea and update send button
        this.userInput.addEventListener('input', () => {
            this.autoResizeTextarea();
            this.updateSendButtonState();
        });

        // Image upload handling
        this.imageUpload.addEventListener('change', (e) => this.handleImageUpload(e));

        // Clear image preview
        this.clearImagePreview.addEventListener('click', () => this.clearImage());
    }

    autoResizeTextarea() {
        const textarea = this.userInput;
        textarea.style.height = 'auto';
        const scrollHeight = textarea.scrollHeight;
        const maxHeight = 120;
        
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
            this.imagePreviewContainer.classList.add('visible');
        };
        reader.readAsDataURL(file);
    }

    clearImage() {
        this.imagePreviewContainer.classList.remove('visible');
        setTimeout(() => {
            this.imagePreview.src = '';
        }, 300);
        
        this.selectedImageFile = null;
        this.imageUpload.value = '';
        this.updateSendButtonState();
    }

    showChatUI() {
        // Hide welcome screen and show chat interface
        this.welcomeScreen.classList.add('hidden');
        
        setTimeout(() => {
            this.welcomeScreen.style.display = 'none';
            this.chatContainer.classList.add('visible');
            
            // Show input area after chat container is visible
            setTimeout(() => {
                const inputArea = document.getElementById('input-area');
                inputArea.classList.add('visible');
            }, 200);
        }, 500);
    }

    async sendMessage() {
        const text = this.userInput.value.trim();
        const image = this.selectedImageFile;

        if ((!text && !image) || this.isLoading) return;

        // Show chat UI if this is the first message
        if (this.messages.length === 0) {
            this.showChatUI();
        }

        // Hide any error messages
        this.hideError();

        // Create user message object
        const userMessage = {
            role: 'user',
            parts: []
        };

        if (text) {
            userMessage.parts.push({ text: text });
        }

        // Add user message to conversation history
        this.messages.push(userMessage);

        // Render user message in UI
        this.renderMessage(userMessage, image);

        // Clear input and image
        this.userInput.value = '';
        this.clearImage();
        this.autoResizeTextarea();

        // Show loading indicator
        this.showLoadingIndicator();

        try {
            // Prepare form data for backend
            const formData = new FormData();
            
            // Add conversation history
            formData.append('messages', JSON.stringify(this.messages));
            
            // Add current prompt text
            if (text) {
                formData.append('prompt', text);
            }
            
            // Add image if present
            if (image) {
                formData.append('image', image);
            }

            // Send request to backend
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

            // Create AI response message
            const aiMessage = {
                role: 'model',
                parts: [{ text: data.response }]
            };

            // Add to conversation history
            this.messages.push(aiMessage);

            // Render AI message in UI
            this.renderMessage(aiMessage);
            
        } catch (error) {
            console.error('Error sending message:', error);
            this.showError('Gagal mendapatkan respons dari Gemini. Silakan coba lagi.');
        } finally {
            this.hideLoadingIndicator();
        }
    }

    renderMessage(message, imageFile = null) {
        const messageDiv = document.createElement('div');
        messageDiv.className = `message ${message.role}-message`;

        let content = '';

        // Add image if present (for user messages)
        if (imageFile && message.role === 'user') {
            const imageUrl = URL.createObjectURL(imageFile);
            content += `<img src="${imageUrl}" alt="Uploaded image">`;
        }

        // Add text content
        message.parts.forEach(part => {
            if (part.text) {
                if (message.role === 'model') {
                    content += this.formatAIResponse(part.text);
                } else {
                    content += this.escapeHtml(part.text);
                }
            }
        });

        messageDiv.innerHTML = content;
        this.chatContainer.appendChild(messageDiv);
        this.scrollToBottom();
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
        this.loadingIndicator.classList.add('visible');
        this.updateSendButtonState();
        this.scrollToBottom();
    }

    hideLoadingIndicator() {
        this.isLoading = false;
        this.loadingIndicator.classList.remove('visible');
        this.updateSendButtonState();
    }

    showError(message) {
        this.errorMessageArea.textContent = message;
        this.errorMessageArea.classList.add('visible');
        this.scrollToBottom();

        // Auto-hide error after 5 seconds
        setTimeout(() => {
            this.hideError();
        }, 5000);
    }

    hideError() {
        this.errorMessageArea.classList.remove('visible');
        setTimeout(() => {
            this.errorMessageArea.textContent = '';
        }, 300);
    }

    scrollToBottom() {
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