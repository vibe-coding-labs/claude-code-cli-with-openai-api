.PHONY: build run clean test help install uninstall

# Binary name
BINARY_NAME=claude-with-openai-api
# Installation directory (can be overridden)
INSTALL_DIR?=~/.local/bin
# System installation directory (requires sudo)
SYSTEM_INSTALL_DIR=/usr/local/bin

# Default target
all: build

# Build the application
build:
	@echo "Building claude-with-openai-api..."
	@go build -o claude-with-openai-api main.go
	@echo "Build complete!"

# Run the application
run: build
	@echo "Starting server..."
	@./claude-with-openai-api

# Run without building (use existing binary)
start:
	@echo "Starting server..."
	@./claude-with-openai-api

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f claude-with-openai-api
	@echo "Clean complete!"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies downloaded!"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Format complete!"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Install the binary to PATH
install: build
	@echo "Installing $(BINARY_NAME)..."
	@if [ ! -f ./$(BINARY_NAME) ]; then \
		echo "❌ Error: Binary $(BINARY_NAME) not found. Please run 'make build' first."; \
		exit 1; \
	fi
	@# Expand tilde in INSTALL_DIR
	@INSTALL_PATH=$$(echo $(INSTALL_DIR) | sed "s|^~|$$HOME|"); \
	PARENT_DIR=$$(dirname "$$INSTALL_PATH"); \
	\
	echo "📦 Target directory: $$INSTALL_PATH"; \
	\
	# Try to create and use user directory \
	if mkdir -p "$$INSTALL_PATH" 2>/dev/null && [ -w "$$INSTALL_PATH" ] 2>/dev/null; then \
		cp ./$(BINARY_NAME) "$$INSTALL_PATH/$(BINARY_NAME)"; \
		chmod +x "$$INSTALL_PATH/$(BINARY_NAME)"; \
		echo "✅ Installed $(BINARY_NAME) to $$INSTALL_PATH/$(BINARY_NAME)"; \
		echo ""; \
		\
		# Check if directory is in PATH \
		if echo "$$PATH" | grep -qE "(^|:)$$INSTALL_PATH(:|$$)"; then \
			echo "✅ $$INSTALL_PATH is already in your PATH."; \
			echo "   You can now run '$(BINARY_NAME)' from anywhere."; \
		else \
			echo "⚠️  Warning: $$INSTALL_PATH is not in your PATH."; \
			echo ""; \
			echo "   To add it to your PATH, run:"; \
			if [ -f "$$HOME/.zshrc" ]; then \
				echo "   echo 'export PATH=\"$$INSTALL_PATH:\$$PATH\"' >> ~/.zshrc"; \
				echo "   source ~/.zshrc"; \
			elif [ -f "$$HOME/.bashrc" ]; then \
				echo "   echo 'export PATH=\"$$INSTALL_PATH:\$$PATH\"' >> ~/.bashrc"; \
				echo "   source ~/.bashrc"; \
			else \
				echo "   export PATH=\"$$INSTALL_PATH:\$$PATH\""; \
				echo "   (Add this to your shell profile file)"; \
			fi; \
		fi; \
	else \
		echo "⚠️  Cannot write to $$INSTALL_PATH (permission denied)"; \
		echo ""; \
		echo "   Options:"; \
		echo "   1. Install to system directory (requires sudo):"; \
		echo "      make install-system"; \
		echo ""; \
		echo "   2. Install to a custom directory:"; \
		echo "      make install INSTALL_DIR=/path/to/your/bin"; \
		echo ""; \
		echo "   3. Manually copy the binary:"; \
		echo "      sudo cp ./$(BINARY_NAME) $(SYSTEM_INSTALL_DIR)/$(BINARY_NAME)"; \
		echo "      sudo chmod +x $(SYSTEM_INSTALL_DIR)/$(BINARY_NAME)"; \
		exit 1; \
	fi

# Install to system directory (requires sudo)
install-system: build
	@echo "Installing $(BINARY_NAME) to system directory..."
	@if [ ! -f ./$(BINARY_NAME) ]; then \
		echo "❌ Error: Binary $(BINARY_NAME) not found. Please run 'make build' first."; \
		exit 1; \
	fi
	@sudo cp ./$(BINARY_NAME) $(SYSTEM_INSTALL_DIR)/$(BINARY_NAME)
	@sudo chmod +x $(SYSTEM_INSTALL_DIR)/$(BINARY_NAME)
	@echo "✅ Installed $(BINARY_NAME) to $(SYSTEM_INSTALL_DIR)/$(BINARY_NAME)"
	@echo "   You can now run '$(BINARY_NAME)' from anywhere."

# Uninstall the binary from PATH
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@INSTALL_PATH=$$(echo $(INSTALL_DIR) | sed "s|^~|$$HOME|"); \
	USER_INSTALL="$$INSTALL_PATH/$(BINARY_NAME)"; \
	SYS_INSTALL="$(SYSTEM_INSTALL_DIR)/$(BINARY_NAME)"; \
	FOUND=0; \
	\
	if [ -f "$$USER_INSTALL" ]; then \
		rm -f "$$USER_INSTALL"; \
		echo "✅ Removed $$USER_INSTALL"; \
		FOUND=1; \
	fi; \
	\
	if [ -f "$$SYS_INSTALL" ]; then \
		sudo rm -f "$$SYS_INSTALL"; \
		echo "✅ Removed $$SYS_INSTALL"; \
		FOUND=1; \
	fi; \
	\
	if [ $$FOUND -eq 0 ]; then \
		echo "⚠️  $(BINARY_NAME) not found in common installation directories."; \
		echo ""; \
		echo "   Searched:"; \
		echo "     - $$USER_INSTALL"; \
		echo "     - $$SYS_INSTALL"; \
		echo ""; \
		echo "   To remove from a custom location:"; \
		echo "     make uninstall INSTALL_DIR=/path/to/your/bin"; \
		echo ""; \
		echo "   Or manually remove:"; \
		echo "     which $(BINARY_NAME)"; \
		echo "     rm \$$(which $(BINARY_NAME))"; \
	fi

# Display help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  run           - Build and run the application"
	@echo "  start         - Run existing binary"
	@echo "  clean         - Remove build artifacts"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  fmt           - Format code"
	@echo "  test          - Run tests"
	@echo "  install       - Install binary to ~/.local/bin (or system directory if needed)"
	@echo "  install-system - Install binary to /usr/local/bin (requires sudo)"
	@echo "  uninstall     - Remove binary from PATH"
	@echo "  help          - Show this help message"

