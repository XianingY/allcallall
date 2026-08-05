package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/apperror"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/tasksched"
)

// TaskSchedulerHandler 暴露周期（weekly）任务的 HTTP 接口。
type TaskSchedulerHandler struct {
	logger  zerolog.Logger
	service *tasksched.Service
	metrics metrics.Recorder
}

// NewTaskSchedulerHandler 构造处理器
func NewTaskSchedulerHandler(log zerolog.Logger, service *tasksched.Service, recorder metrics.Recorder) *TaskSchedulerHandler {
	return &TaskSchedulerHandler{
		logger:  log.With().Str("component", "task_scheduler_handler").Logger(),
		service: service,
		metrics: recorder,
	}
}

// RegisterRoutes 注册受保护路由（需在 AuthMiddleware 之后挂载）
func (h *TaskSchedulerHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/tasks", h.handleCreate)
	rg.GET("/tasks", h.handleList)
	rg.GET("/tasks/:id", h.handleGet)
	rg.POST("/tasks/:id/pause", h.handlePause)
	rg.POST("/tasks/:id/resume", h.handleResume)
	rg.POST("/tasks/:id/trigger", h.handleTrigger)
	rg.GET("/tasks/:id/runs", h.handleListRuns)
}

type createTaskRequest struct {
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	OrgID         *uint64 `json:"org_id"`
	Timezone      string  `json:"timezone"`
	Weekdays      []int   `json:"weekdays"`
	RunTimeOfDay  string  `json:"run_time_of_day"`
	IntervalWeeks int     `json:"interval_weeks"`
	MaxFailures   int     `json:"max_failures"`
}

func (h *TaskSchedulerHandler) handleCreate(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	task, err := h.service.Create(c.Request.Context(), claims.UserID, tasksched.CreateInput{
		Title:         req.Title,
		Description:   req.Description,
		OrgID:         req.OrgID,
		Timezone:      req.Timezone,
		Weekdays:      req.Weekdays,
		RunTimeOfDay:  req.RunTimeOfDay,
		IntervalWeeks: req.IntervalWeeks,
		MaxFailures:   req.MaxFailures,
	})
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"task": task})
}

func (h *TaskSchedulerHandler) handleList(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	tasks, err := h.service.List(c.Request.Context(), claims.UserID)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"tasks": tasks})
}

func (h *TaskSchedulerHandler) handleGet(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := h.service.Get(c.Request.Context(), claims.UserID, id)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"task": task})
}

func (h *TaskSchedulerHandler) handlePause(c *gin.Context) {
	h.mutateTask(c, h.service.Pause)
}

func (h *TaskSchedulerHandler) handleResume(c *gin.Context) {
	h.mutateTask(c, h.service.Resume)
}

func (h *TaskSchedulerHandler) handleTrigger(c *gin.Context) {
	h.mutateTask(c, h.service.Trigger)
}

func (h *TaskSchedulerHandler) handleListRuns(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	limit := uint64(50)
	if l := c.Query("limit"); l != "" {
		if v, e := strconv.ParseUint(l, 10, 32); e == nil {
			limit = v
		}
	}
	runs, err := h.service.ListRuns(c.Request.Context(), claims.UserID, id, limit)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"runs": runs})
}

// mutateTask 统一处理需要 owner 校验的变更类操作（pause/resume/trigger）。
func (h *TaskSchedulerHandler) mutateTask(c *gin.Context, fn func(ctx context.Context, owner, id uint64) error) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	if err := fn(c.Request.Context(), claims.UserID, id); err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"status": "ok"})
}

// writeAppError 把业务错误（含 *apperror.AppError）映射为 HTTP 响应。
func writeAppError(c *gin.Context, err error) {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		JSONErrorWithCode(c, appErr.HTTPStatus, appErr.Code, appErr.Message)
		return
	}
	JSONError(c, http.StatusInternalServerError, "internal error")
}
