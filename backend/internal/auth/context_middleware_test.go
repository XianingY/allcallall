package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/trace"
)

func TestClaimsContextRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	claims := &Claims{UserID: 7, Email: "alice@example.com"}

	SetClaimsToContext(ctx, claims)
	got, err := GetClaimsFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != claims {
		t.Fatalf("unexpected claims pointer: got=%p want=%p", got, claims)
	}
}

func TestGetClaimsFromContextErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	if _, err := GetClaimsFromContext(ctx); err == nil {
		t.Fatal("expected missing claims error")
	}

	ctx.Set(ginUserKey, "invalid")
	if _, err := GetClaimsFromContext(ctx); err == nil {
		t.Fatal("expected invalid type error")
	}
}

func TestExtractToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "empty", header: "", want: ""},
		{name: "bearer", header: "Bearer abc.def", want: "abc.def"},
		{name: "lowercase bearer", header: "bearer token-1", want: "token-1"},
		{name: "wrong scheme", header: "Basic token", want: ""},
		{name: "missing token", header: "Bearer", want: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := extractToken(tc.header); got != tc.want {
				t.Fatalf("unexpected token: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mgr, err := NewManager(Config{Secret: "secret", Issuer: "allcallall"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token, err := mgr.GenerateAccessToken(42, "alice@example.com")
	if err != nil {
		t.Fatalf("generate token failed: %v", err)
	}

	t.Run("missing token", func(t *testing.T) {
		router := authRouterWithRequestID("req-auth-missing-1")
		router.GET("/secure", Middleware(mgr), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
		assertAuthError(t, rec, authTokenMissingCode, "missing bearer token", "req-auth-missing-1")
	})

	t.Run("invalid token", func(t *testing.T) {
		router := authRouterWithRequestID("req-auth-invalid-1")
		router.GET("/secure", Middleware(mgr), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
		assertAuthError(t, rec, authTokenInvalidCode, "invalid token", "req-auth-invalid-1")
	})

	t.Run("header token", func(t *testing.T) {
		router := gin.New()
		router.GET("/secure", Middleware(mgr), func(c *gin.Context) {
			claims, err := GetClaimsFromContext(c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			c.JSON(http.StatusOK, gin.H{"email": claims.Email, "user_id": claims.UserID})
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("query token", func(t *testing.T) {
		router := gin.New()
		router.GET("/secure", Middleware(mgr), func(c *gin.Context) {
			claims, err := GetClaimsFromContext(c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			c.JSON(http.StatusOK, gin.H{"email": claims.Email, "user_id": claims.UserID})
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/secure?token="+token, nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})
}

func authRouterWithRequestID(requestID string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("X-Request-ID", requestID)
		c.Request = c.Request.WithContext(trace.WithRequestID(c.Request.Context(), requestID))
		c.Next()
	})
	return router
}

func assertAuthError(t *testing.T, rec *httptest.ResponseRecorder, code string, message string, requestID string) {
	t.Helper()

	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	if got["error"] != message {
		t.Fatalf("unexpected error payload: %+v", got)
	}
	if got["code"] != code {
		t.Fatalf("unexpected code payload: %+v", got)
	}
	if got["request_id"] != requestID {
		t.Fatalf("unexpected request_id payload: %+v", got)
	}
	if got["success"] != false {
		t.Fatalf("unexpected success payload: %+v", got)
	}
}
