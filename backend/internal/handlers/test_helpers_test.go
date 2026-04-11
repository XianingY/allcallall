package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/user"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type handlerTestEnv struct {
	db         *gorm.DB
	mock       sqlmock.Sqlmock
	userSvc    *user.Service
	contactSvc *contact.Service
	presence   *presence.Manager
	jwtMgr     *auth.Manager
	logger     zerolog.Logger
}

func newHandlerTestEnv(t *testing.T) *handlerTestEnv {
	t.Helper()

	miniRedis, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		miniRedis.Close()
		t.Fatalf("create sqlmock failed: %v", err)
	}

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = db.Close()
		miniRedis.Close()
		t.Fatalf("open gorm db failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		miniRedis.Close()
	})

	userSvc := user.NewService(user.NewRepository(gdb))
	contactSvc := contact.NewService(contact.NewRepository(gdb), userSvc)
	presenceClient := redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
	presenceMgr := presence.NewManager(presenceClient, zerolog.Nop(), userSvc)
	jwtMgr, err := auth.NewManager(auth.Config{Secret: "secret", Issuer: "allcallall"})
	if err != nil {
		t.Fatalf("create jwt manager failed: %v", err)
	}

	return &handlerTestEnv{
		db:         gdb,
		mock:       mock,
		userSvc:    userSvc,
		contactSvc: contactSvc,
		presence:   presenceMgr,
		jwtMgr:     jwtMgr,
		logger:     zerolog.Nop(),
	}
}

func newRouterWithClaims(claims *auth.Claims, register func(*gin.RouterGroup)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if claims != nil {
		router.Use(func(c *gin.Context) {
			auth.SetClaimsToContext(c, claims)
			c.Next()
		})
	}
	register(router.Group("/api/v1"))
	return router
}

func performRequest(t *testing.T, router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, body []byte, out any) {
	t.Helper()

	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	return string(hash)
}

func userRows(id uint64, email, passwordHash, displayName, fcmToken string, createdAt, updatedAt, lastSeen any) *sqlmock.Rows {
	cols := []string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}
	return sqlmock.NewRows(cols).AddRow(id, email, passwordHash, displayName, fcmToken, createdAt, updatedAt, lastSeen)
}

func expectHandlerStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()

	if rec.Code != want {
		t.Fatalf("unexpected status: got=%d want=%d body=%s", rec.Code, want, rec.Body.String())
	}
}
