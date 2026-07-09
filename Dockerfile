# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npx vite build

# Stage 2: Build backend
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/vlesspanel .

# Stage 3: Runtime
FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata

COPY --from=backend-builder /app/vlesspanel /app/vlesspanel
COPY --from=frontend-builder /app/frontend/dist /app/static

ENV VLESSPANEL_PORT=8080
ENV VLESSPANEL_AGGREGATOR_DIR=/data/aggregator
ENV VLESSPANEL_PANELS_FILE=/data/panels.json
ENV VLESSPANEL_STATIC_DIR=/app/static

EXPOSE 8080
CMD ["/app/vlesspanel"]
