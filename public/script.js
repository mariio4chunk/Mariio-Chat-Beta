class GeminiChat {
    constructor() {
        this.messages = [];
        this.isLoading = false;
        this.currentImage = null;
        
        this.initializeElements();
        this.attachEventListeners();
        this.autoResizeTextarea();
    }

    initializeElements() {
        this.chatContainer = document.getElementById('chat-container');
        this.userInput = document.getElementById('user-input');
        this.sendButton = document.getElementById('send-button');
        this.imageUpload = document.getElementById('image-upload');
        this.imagePreview = document.getElementById('image-preview');
        this.typingIndicator = document.getElementById('typing-indicator');
    }

    attachEventListeners() {
        // Send button click
        this.sendButton.addEventListener('click', () => this.handleSendMessage());

        // Enter key press (with Shift+Enter for new line)
        this.userInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.handleSendMessage();
            }
        });

        // Image upload
        this.imageUpload.addEventListener('change', (e) => this.handleImageUpload(e));

        // Auto-resize textarea
        this.userInput.addEventListener('input', () => this.autoResizeTextarea());
    }

    autoResizeTextarea() {
        const textarea = this.userInput;
        textarea.style.height = 'auto';
        const scrollHeight = textarea.scrollHeight;
        const maxHeight = 200; // Max height in pixels
        
        if (scrollHeight <= maxHeight) {
            textarea.style.height = `${scrollHeight}px`;
        } else {
            textarea.style.height = `${maxHeight}px`;
        }

        // Update send button state
        this.updateSendButtonState();
    }

    updateSendButtonState() {
        const hasText = this.userInput.value.trim().length > 0;
        const hasImage = this.currentImage !== null;
        
        this.sendButton.disabled = !hasText && !hasImage || this.isLoading;
    }

    handleImageUpload(event) {
        const file = event.target.files[0];
        if (!file) return;

        // Validate file type
        if (!file.type.startsWith('image/')) {
            this.showError('Please select a valid image file.');
            return;
        }

        // Validate file size (10MB max)
        if (file.size > 10 * 1024 * 1024) {
            this.showError('Image file size must be less than 10MB.');
            return;
        }

        this.currentImage = file;
        this.showImagePreview(file);
        this.updateSendButtonState();
    }

    showImagePreview(file) {
        const reader = new FileReader();
        reader.onload = (e) => {
            this.imagePreview.innerHTML = `
                <div class="preview-container">
                    <img src="${e.target.result}" alt="Preview" class="preview-image">
                    <button class="remove-preview" onclick="geminiChat.removeImagePreview()" title="Remove image">
                        ×
                    </button>
                </div>
            `;
            this.imagePreview.classList.add('show');
        };
        reader.readAsDataURL(file);
    }

    removeImagePreview() {
        this.imagePreview.classList.remove('show');
        setTimeout(() => {
            this.imagePreview.innerHTML = '';
        }, 300);
        
        this.currentImage = null;
        this.imageUpload.value = '';
        this.updateSendButtonState();
    }

    async handleSendMessage() {
        const text = this.userInput.value.trim();
        const image = this.currentImage;

        if (!text && !image || this.isLoading) return;

        // Add user message to chat
        this.addUserMessage(text, image);

        // Clear input
        this.userInput.value = '';
        this.removeImagePreview();
        this.autoResizeTextarea();

        // Show typing indicator
        this.showTypingIndicator();

        try {
            // Send message to backend
            const response = await this.sendToGemini(text, image);
            
            if (response.error) {
                throw new Error(response.error);
            }

            // Add AI response to chat
            this.addAIMessage(response.response);
            
        } catch (error) {
            console.error('Error sending message:', error);
            this.showError('Failed to get response from Gemini. Please try again.');
        } finally {
            this.hideTypingIndicator();
        }
    }

    addUserMessage(text, image) {
        const messageElement = document.createElement('div');
        messageElement.className = 'message user';

        let imageHtml = '';
        if (image) {
            const imageUrl = URL.createObjectURL(image);
            imageHtml = `<img src="${imageUrl}" alt="Uploaded image" class="message-image">`;
        }

        messageElement.innerHTML = `
            <div class="message-content">
                ${imageHtml}
                ${text ? `<div>${this.escapeHtml(text)}</div>` : ''}
            </div>
        `;

        this.chatContainer.appendChild(messageElement);
        this.scrollToBottom();

        // Add to messages array for context
        const parts = [];
        if (text) parts.push(text);
        
        this.messages.push({
            role: 'user',
            parts: parts
        });
    }

    addAIMessage(text) {
        const messageElement = document.createElement('div');
        messageElement.className = 'message ai';

        messageElement.innerHTML = `
            <div class="message-content">
                ${this.formatAIResponse(text)}
            </div>
        `;

        this.chatContainer.appendChild(messageElement);
        this.scrollToBottom();

        // Add to messages array for context
        this.messages.push({
            role: 'model',
            parts: [text]
        });
    }

    formatAIResponse(text) {
        // Simple formatting for AI responses
        let formatted = this.escapeHtml(text);
        
        // Convert line breaks to HTML
        formatted = formatted.replace(/\n/g, '<br>');
        
        // Make text bold when surrounded by **
        formatted = formatted.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
        
        // Make text italic when surrounded by *
        formatted = formatted.replace(/\*(.*?)\*/g, '<em>$1</em>');
        
        return formatted;
    }

    async sendToGemini(text, image) {
        const formData = new FormData();
        
        // Add messages history
        formData.append('messages', JSON.stringify(this.messages.concat([{
            role: 'user',
            parts: text ? [text] : []
        }])));

        // Add image if present
        if (image) {
            formData.append('image', image);
        }

        const response = await fetch('/api/chat', {
            method: 'POST',
            body: formData
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        return await response.json();
    }

    showTypingIndicator() {
        this.isLoading = true;
        this.typingIndicator.classList.add('show');
        this.updateSendButtonState();
        this.scrollToBottom();
    }

    hideTypingIndicator() {
        this.isLoading = false;
        this.typingIndicator.classList.remove('show');
        this.updateSendButtonState();
    }

    showError(message) {
        const errorElement = document.createElement('div');
        errorElement.className = 'error-message';
        errorElement.textContent = message;
        
        this.chatContainer.appendChild(errorElement);
        this.scrollToBottom();

        // Remove error message after 5 seconds
        setTimeout(() => {
            if (errorElement.parentNode) {
                errorElement.parentNode.removeChild(errorElement);
            }
        }, 5000);
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

// Initialize the chat application
const geminiChat = new GeminiChat();

// Remove welcome message when first message is sent
const originalAddUserMessage = geminiChat.addUserMessage.bind(geminiChat);
let isFirstMessage = true;

geminiChat.addUserMessage = function(text, image) {
    if (isFirstMessage) {
        const welcomeMessage = document.querySelector('.welcome-message');
        if (welcomeMessage) {
            welcomeMessage.style.opacity = '0';
            welcomeMessage.style.transform = 'translateY(-20px)';
            setTimeout(() => {
                welcomeMessage.remove();
            }, 300);
        }
        isFirstMessage = false;
    }
    return originalAddUserMessage(text, image);
};
