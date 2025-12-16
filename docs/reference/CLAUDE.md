# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AllCallAll is a real-time audio/video communication platform built with:
- **Backend**: Go 1.22+, Gin framework, Pion WebRTC v4.0.0, MySQL 8.0, Redis 7.2
- **Mobile**: React Native 0.74+ with Expo 51.0+, TypeScript, react-native-webrtc
- **Infrastructure**: Docker Compose for local development

## Common Development Commands

### Backend Development

```bash
# Start database services (MySQL + Redis)
./start.sh

# Set environment variables and start backend
cd backend
export CONFIG_PATH=./configs/config.yaml
export MAIL_PASSWORD="your_qq_mail_auth_code"  # Required for email verification
go run cmd/server/main.go

# Verify backend is running
curl http://localhost:8080/health
```

### Mobile Development (Recommended: ADB Reverse Forwarding)

```bash
# Start mobile app with automated script (RECOMMENDED)
cd mobile
bash scripts/dev-client-debug.sh

# Or manually configure:
adb reverse tcp:8080 tcp:8080  # Backend API
adb reverse tcp:8081 tcp:8081  # Metro bundler
npm run start:dev-client

# Alternative: LAN mode (Wi-Fi debugging)
npm run start:dev-client:lan

# Build and install custom development client
npm run android

# Lint code
npm run lint
```

### Database Management

```bash
# View database service status
docker-compose -f infra/docker-compose.yml ps

# View logs
docker-compose -f infra/docker-compose.yml logs -f mysql
docker-compose -f infra/docker-compose.yml logs -f redis

# Stop services
docker-compose -f infra/docker-compose.yml down
```

## Architecture Overview

### Backend Structure (Go)

**Entry Point**: `backend/cmd/server/main.go`
- Initializes configuration from YAML
- Sets up MySQL + Redis connections
- Auto-migrates database models
- Initializes WebRTC media engine (Pion)
- Starts HTTP server on port 8080

**Key Internal Packages**:
- `internal/auth` - JWT authentication and middleware
- `internal/user` - User management (repository + service pattern)
- `internal/contact` - Contact management
- `internal/signaling` - WebRTC signaling hub with WebSocket support
- `internal/media` - Pion WebRTC media engine
- `internal/presence` - Online status management with Redis
- `internal/mail` - Email verification service (QQ SMTP)
- `internal/models` - GORM database models
- `internal/handlers` - HTTP handlers (Gin)
- `internal/database` - MySQL connection via GORM
- `internal/cache` - Redis client

**Configuration**: `backend/configs/config.yaml`
- Server settings (host, port, timeouts)
- Database DSN
- Redis connection
- SMTP mail settings (requires `MAIL_PASSWORD` env var)
- JWT secrets
- ICE servers for WebRTC

### Mobile Structure (React Native/Expo)

**Key Directories**:
- `src/screens/` - Application screens/pages
- `src/components/` - Reusable UI components
- `src/context/` - React Context (AuthContext, SignalingContext)
- `src/navigation/` - React Navigation configuration
- `src/config/` - API endpoints and app configuration
- `src/api/` - API client and HTTP requests

**Context Providers**:
- `AuthContext.tsx` - User authentication state
- `SignalingContext.tsx` - WebSocket signaling and WebRTC session management

**Configuration**: `mobile/src/config/index.ts`
- API endpoints (dev: 192.168.31.217, prod: allcall.cn or 81.68.168.207)
- WebSocket URL for signaling
- Currently hardcoded to production IP (line 25: `const __DEV__ = false`)

### Infrastructure

**Docker Compose**: `infra/docker-compose.yml`
- MySQL 8.0 with health checks
- Redis 7.2 with health checks
- Backend service (optional, for containerized deployment)

## API Endpoints

### REST APIs (prefix: `/api/v1`)

```
POST   /auth/register         - User registration with email verification
POST   /auth/login            - User login (returns JWT)
POST   /email/send-verification-code - Send email verification code
GET    /users/contacts        - Get contact list (auth required)
GET    /users/presence        - Get user online status (auth required)
GET    /users/search          - Search users (auth required)
GET    /ping                  - Health check
```

### WebSocket

```
GET    /api/v1/ws             - WebSocket for signaling (auth via JWT query param)
```

## Development Workflow

### 1. Initial Setup

```bash
# Clone and install dependencies
git clone <repo>
cd allcall

# Backend
cd backend && go mod download && cd ..

# Mobile
cd mobile && npm install && cd ..
```

### 2. Start Development Environment

```bash
# 1. Start databases
./start.sh

# 2. Configure email (one-time setup)
cd backend
cp .env.example .env
# Edit .env and add your QQ Mail authorization code to MAIL_PASSWORD
export MAIL_PASSWORD="xxxx xxxx xxxx xxxx"
export CONFIG_PATH=./configs/config.yaml

# 3. Start backend (terminal 1)
go run cmd/server/main.go

# 4. Start mobile app (terminal 2)
cd mobile
bash scripts/dev-client-debug.sh
```

### 3. Development Notes

**ADB Reverse Forwarding**:
- Mobile app uses `localhost:8080` for API (configured in mobile/src/config/index.ts)
- ADB automatically forwards `localhost:8080` → device's `localhost:8080`
- This provides stable connection better than LAN IP

**Virtual Metro Entry Point**:
- `.expo/.virtual-metro-entry.js` is critical - never delete
- The dev-client-debug.sh script automatically protects this file

**Email Verification**:
- Uses QQ Mail SMTP (smtp.qq.com:587)
- Requires QQ Mail authorization code (not password)
- Set via `MAIL_PASSWORD` environment variable

## Key Configuration Files

- `backend/configs/config.yaml` - Backend configuration (DB, Redis, SMTP, JWT)
- `mobile/src/config/index.ts` - Mobile API endpoints (dev/prod switching)
- `infra/docker-compose.yml` - Database services configuration
- `backend/.env` - Environment variables (MAIL_PASSWORD, etc.)

## Common Issues & Solutions

### Physical device cannot connect to backend
1. Check ADB reverse: `adb reverse --list`
2. Verify backend: `curl http://localhost:8080/health`
3. Clear app data: `adb shell pm clear com.allcallall.mobile`
4. Re-run: `bash mobile/scripts/dev-client-debug.sh`

### Metro compilation fails
1. Use the automated script (handles virtual entry point)
2. Manual fix: preserve `.expo/.virtual-metro-entry.js` during cache clean

### Backend won't start (MySQL connection error)
1. Ensure databases running: `./start.sh`
2. Check MySQL: `docker-compose -f infra/docker-compose.yml ps`

### Email verification not working
1. Verify `MAIL_PASSWORD` set: `echo $MAIL_PASSWORD`
2. Test endpoint:
   ```bash
   curl -X POST http://localhost:8080/api/v1/email/send-verification-code \
     -H "Content-Type: application/json" \
     -d '{"email":"test@example.com"}'
   ```

## Testing

No formal test suite found. To test manually:
- Backend: Use curl commands to REST endpoints
- WebSocket: Use wscat or similar tool: `wscat -c ws://localhost:8080/api/v1/ws?token=<JWT>`
- Mobile: Physical device testing via ADB debugging

## Database Schema

Auto-migrated models (via GORM in main.go):
- `User` - User accounts
- `Contact` - Contact relationships
- `EmailVerificationCode` - Email verification codes
- `EmailSendLog` - Email sending logs

## WebRTC Architecture

**Backend**:
- Pion v4.0.0 WebRTC engine initialized in main.go
- Signaling hub manages WebSocket connections
- ICE servers: Google STUN (stun.l.google.com:19302)

**Mobile**:
- react-native-webrtc for peer connections
- SignalingContext manages WebSocket + WebRTC session lifecycle
- Connects to backend at `/api/v1/ws` for signaling

## Environment-Specific Configuration

**Development**:
- Mobile config: `mobile/src/config/index.ts` line 23: `const __DEV__ = false`
- To use dev config, change to `true` (uses 192.168.31.217)
- ADB forwarding required for localhost

**Production**:
- Backend: Set production secrets (JWT_SECRET, MAIL_PASSWORD, etc.)
- Mobile: Uses production IP (81.68.168.207) or domain (allcall.cn)
- WebSocket must use wss:// for HTTPS

## Important Files

- `start.sh` - Quick database startup script
- `mobile/scripts/dev-client-debug.sh` - Mobile dev setup (ADB + Metro)
- `backend/cmd/server/main.go` - Backend entry point
- `backend/configs/config.yaml` - Backend config
- `infra/docker-compose.yml` - Local database services
- `mobile/src/config/index.ts` - Mobile API configuration
- `mobile/src/context/SignalingContext.tsx` - WebRTC signaling logic
- `mobile/src/context/AuthContext.tsx` - Authentication state

## Branch Strategy

- `main` - Stable release version
- `develop` - Development branch
- `feature/*` - Feature branches
- `bugfix/*` - Bug fix branches

## Technology Choices

- **Go**: High-performance backend with excellent WebRTC support (Pion)
- **Gin**: Fast HTTP framework
- **GORM**: ORM for MySQL with auto-migration
- **Redis**: Session management, presence tracking, caching
- **React Native + Expo**: Cross-platform mobile with excellent WebRTC support
- **Pion WebRTC**: Pure Go WebRTC implementation (no C dependencies)
- **Gorilla WebSocket**: Robust WebSocket implementation
