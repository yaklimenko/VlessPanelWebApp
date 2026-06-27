.PHONY: build run dev-front clean

build: build-backend build-frontend

build-backend:
	@echo "🔨 Building Go backend..."
	/tmp/go/bin/go build -o build/vlesspanel ./backend
	@echo "✅ Backend built: build/vlesspanel"

build-frontend:
	@echo "🔨 Building React frontend..."
	cd frontend && npx vite build
	@echo "✅ Frontend built: frontend/dist/"

run:
	@echo "🚀 Starting VlessPanel on :8080..."
	cd backend && /tmp/go/bin/go run .

dev-front:
	@echo "🎨 Starting frontend dev server..."
	cd frontend && npx vite --host

clean:
	@echo "🧹 Cleaning..."
	rm -rf build/vlesspanel frontend/dist
	@echo "✅ Clean"
