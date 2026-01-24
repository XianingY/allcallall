# AllCallAll Agentic Guidelines

This document provides context, commands, and conventions for AI agents operating in the `allcallall` repository.
Follow these guidelines to ensure consistency, stability, and high-quality code contributions.

## 1. Project Overview
- **Architecture**: Monorepo containing a Go backend and a React Native mobile app.
- **Backend**: Go 1.22+, Gin framework, Gorm (MySQL), Redis, Pion WebRTC. Located in `backend/`.
- **Mobile**: React Native (0.74+), Expo (SDK 51), TypeScript. Located in `mobile/`.
- **Infrastructure**: Docker Compose for MySQL, Redis, Nginx. Located in `infra/`.
- **Purpose**: Real-time audio/video communication platform with WebRTC, contact management, and offline AI translation.

## 2. Operational Commands

### Backend (Go)
| Action | Command | Notes |
|--------|---------|-------|
| **Run Server** | `make run-backend` | Runs `cmd/server/main.go` on port 8080. |
| **Build** | `cd backend && go build ./...` | Compiles all packages. |
| **Test All** | `cd backend && go test ./...` | Runs all unit tests. |
| **Test Single** | `cd backend && go test -v -run <TestName> <Package>` | Ex: `go test -v -run TestLogin ./internal/auth` |
| **Test Coverage** | `cd backend && go test -cover ./...` | Shows test coverage. |
| **Lint/Format** | `cd backend && gofmt -s -w .` | Format code (run before commit). |
| **Vet** | `cd backend && go vet ./...` | Static analysis. |
| **Deps** | `cd backend && go mod tidy` | Run if imports are changed. |

### Mobile (React Native/Expo)
| Action | Command | Notes |
|--------|---------|-------|
| **Install** | `cd mobile && npm install` | Strict dependency management. |
| **Run Dev** | `cd mobile && npx expo start` | Starts Metro bundler. |
| **Run Android** | `cd mobile && npx expo start --android` | Launch on Android emulator/device. |
| **Run iOS** | `cd mobile && npx expo start --ios` | Launch on iOS simulator (macOS only). |
| **Test All** | `cd mobile && npm test` | Runs Jest tests. |
| **Test Single** | `cd mobile && npx jest -t "<TestName>"` | Run specific test by name. |
| **Test File** | `cd mobile && npx jest src/screens/Login.test.tsx` | Run specific test file. |
| **Lint** | `cd mobile && npm run lint` | Runs ESLint (`.ts, .tsx` files). |
| **TypeScript** | `cd mobile && npx tsc --noEmit` | Type check without emitting files. |
| **Build Android** | `make build-android` | Builds debug APK. |

### Infrastructure
- **Start DBs**: `./scripts/development/start-services.sh` (Starts MySQL/Redis via Docker).
- **Stop DBs**: `docker-compose -f infra/docker-compose.yml down`
- **Setup**: `make setup` (Submodules + npm install).
- **Clean**: `make clean` (Removes build artifacts).

## 3. Code Style & Conventions

### General
- **Pathing**: Always use **absolute paths** when reading/writing files.
- **Conventions**: Adhere strictly to existing patterns. Analyze surrounding code before editing.
- **Safety**: Never commit secrets (`.env` files, credentials).

### Backend (Go)
- **Structure**: Follow `cmd/` for entry points and `internal/` for business logic.
  - `internal/`: Business logic modules (auth, user, contact, signaling, etc.)
  - `cmd/server/`: Entry point (`main.go`)
  - Each module has: `service.go`, `repository.go`, and optionally `handlers.go`
- **Naming**:
  - Exported: `PascalCase` (e.g., `NewService`, `RegisterInput`).
  - Private: `camelCase` (e.g., `verifyPassword`, `hashPassword`).
  - Interfaces: suffix with `er` where appropriate (e.g., `Reader`), or use descriptive names.
  - Struct constructors: Use `NewXxx` pattern (e.g., `NewService`, `NewRepository`).
- **Error Handling**:
  - Explicit checks: `if err != nil { return fmt.Errorf("context: %w", err) }`.
  - Wrap errors to provide context using `%w` for error chains.
  - Define domain errors as package-level `var` (e.g., `var ErrEmailAlreadyUsed = errors.New(...)`).
- **Logging**: Use `zerolog` (via `appLogger`). Do not use `fmt.Println` in production code.
- **Comments**:
  - Existing code uses **bilingual comments** (Chinese/English).
  - **Rule**: If modifying a file with bilingual comments, maintain that style. Otherwise, English is preferred.
  - Example: `// Service 用户业务逻辑` followed by `// Service handles high-level user operations.`
  - Always document exported types, functions, and methods.
- **Imports**: Group in this order:
  1. Standard library (e.g., `context`, `errors`, `strings`)
  2. Third-party packages (e.g., `github.com/gin-gonic/gin`)
  3. Internal packages (e.g., `github.com/allcallall/backend/internal/...`)
- **Context**: Always pass `context.Context` as first parameter in service/repository methods.
- **Database**: Use Gorm ORM. Repository layer handles all DB access; services coordinate business logic.

### Mobile (TypeScript)
- **Structure**: Feature-based organization within `src/`:
  - `screens/`: React Native screens
  - `components/`: Reusable UI components
  - `context/`: React Context providers (e.g., `AuthContext.tsx`)
  - `api/`: API client modules (e.g., `auth.ts`, `users.ts`)
  - `services/`: Native services (e.g., WebRTC, push notifications)
  - `navigation/`: React Navigation configuration
- **Naming**:
  - Components/Files: `PascalCase` (e.g., `AuthContext.tsx`, `LoginScreen.tsx`).
  - Functions/Vars: `camelCase` (e.g., `handleLogin`, `bootstrap`).
  - Constants: `UPPER_SNAKE_CASE` (e.g., `STORAGE_KEY`, `API_URL`).
- **Types**:
  - Use `interface` for object definitions (props, state, API responses).
  - Avoid `any`. Use strict typing (`strict: true` in tsconfig).
  - Define types inline or in dedicated type files.
  - Export types alongside functions when needed.
- **Components**: Functional components with Hooks. Avoid Class components.
  - Use `React.FC<Props>` for component definitions.
  - Prefer `useCallback` and `useMemo` for optimization.
- **Imports**: Order imports as follows:
  1. React / React Native (e.g., `import React from "react"`)
  2. Third-party libraries (e.g., Expo, AsyncStorage)
  3. Internal Components/API (`../api/auth`, `../components/Button`)
  4. Types/Utils (if not inline)
- **Async/Await**: Use async/await instead of Promises for readability.
- **Error Handling**: Wrap API calls and storage operations in try/catch blocks.

## 4. Testing Guidelines
- **Backend**: Tests live alongside code (`user_test.go`). Use `testify` or standard `testing` package.
- **Mobile**: Tests in `__tests__` or alongside files (`*.test.tsx`).
- **Mocking**: Use interfaces to mock dependencies (e.g., Repository layer) for service tests.

## 5. Agent Workflow
1. **Discovery**: Run `ls -F` and `read` to understand the file structure before editing.
2. **Analysis**: Check `go.mod` or `package.json` to verify available libraries.
3. **Verification**: After editing, ALWAYS run the relevant build or test command to ensure no regressions.
   - Go: `cd backend && go build ./...`
   - Go vet: `cd backend && go vet ./...`
   - TS: `cd mobile && npm run lint` or `npx tsc --noEmit`
4. **Communication**: Be concise. Explain *why* a complex change was made, not *what* the code does (the code speaks for itself).

## 6. Critical Rules
- **Do Not Revert**: Do not revert changes unless explicitly instructed or to fix a regression you caused.
- **Secrets**: `backend/.env` is ignored by git. Use `backend/.env.example` as a template.
- **Context**: Use `grep` and `glob` to find usage references before renaming or refactoring.

## 7. Common Patterns

### Backend Code Examples
```go
// Service constructor pattern
func NewService(repo *Repository, deps *OtherService) *Service {
    return &Service{repo: repo, deps: deps}
}

// Context-first pattern in methods
func (s *Service) GetUser(ctx context.Context, id string) (*models.User, error) {
    user, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("get user: %w", err)
    }
    return user, nil
}

// Domain error definitions
var (
    ErrNotFound = errors.New("resource not found")
    ErrInvalidInput = errors.New("invalid input")
)
```

### Mobile Code Examples
```typescript
// Component with proper typing
export const MyComponent: React.FC<{ userId: string }> = ({ userId }) => {
  const [data, setData] = useState<User | null>(null);
  
  const fetchData = useCallback(async () => {
    try {
      const user = await usersApi.getUser(userId);
      setData(user);
    } catch (error) {
      console.error("Failed to fetch user", error);
    }
  }, [userId]);
  
  useEffect(() => {
    fetchData();
  }, [fetchData]);
  
  return <View>...</View>;
};
```

---
*Generated by Antigravity for allcallall*
