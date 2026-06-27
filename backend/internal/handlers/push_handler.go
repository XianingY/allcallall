package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

// PushHandler serves browser/mobile push device registration endpoints.
type PushHandler struct {
	logger zerolog.Logger
	users  *user.Service
}

func NewPushHandler(logger zerolog.Logger, users *user.Service) *PushHandler {
	return &PushHandler{
		logger: logger.With().Str("component", "push_handler").Logger(),
		users:  users,
	}
}

func (h *PushHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	group := protected.Group("/push/devices")
	group.GET("", h.handleListDevices)
	group.POST("", h.handleRegisterDevice)
	group.DELETE("/:id", h.handleDeleteDevice)
}

type registerPushDeviceRequest struct {
	Token      string `json:"token" binding:"required"`
	Provider   string `json:"provider"`
	Platform   string `json:"platform"`
	DeviceName string `json:"device_name"`
	AppVersion string `json:"app_version"`
}

func (h *PushHandler) handleListDevices(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	devices, err := h.users.ListPushDevices(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("list push devices failed")
		JSONError(c, http.StatusInternalServerError, "failed to list push devices")
		return
	}
	response := make([]pushDeviceResponse, 0, len(devices))
	for _, device := range devices {
		response = append(response, toPushDeviceResponse(device))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"devices": response})
}

func (h *PushHandler) handleRegisterDevice(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req registerPushDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	device, err := h.users.RegisterPushDevice(c.Request.Context(), claims.UserID, user.SavePushRegistrationInput{
		Token:      req.Token,
		Provider:   req.Provider,
		Platform:   req.Platform,
		DeviceName: req.DeviceName,
		AppVersion: req.AppVersion,
	})
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("register push device failed")
		JSONError(c, http.StatusInternalServerError, "failed to register push device")
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"message": "push device registered", "device": toPushDeviceResponse(*device)})
}

func (h *PushHandler) handleDeleteDevice(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	deviceID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || deviceID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid push device id")
		return
	}
	if err := h.users.DeletePushDevice(c.Request.Context(), claims.UserID, deviceID); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			JSONError(c, http.StatusNotFound, "push device not found")
			return
		}
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Uint64("device_id", deviceID).Msg("delete push device failed")
		JSONError(c, http.StatusInternalServerError, "failed to delete push device")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": "push device deleted"})
}

type pushDeviceResponse struct {
	ID             uint64 `json:"id"`
	Provider       string `json:"provider"`
	Platform       string `json:"platform"`
	DeviceName     string `json:"device_name"`
	AppVersion     string `json:"app_version"`
	LastRegistered string `json:"last_registered"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toPushDeviceResponse(device models.PushDevice) pushDeviceResponse {
	return pushDeviceResponse{
		ID:             device.ID,
		Provider:       device.Provider,
		Platform:       device.Platform,
		DeviceName:     device.DeviceName,
		AppVersion:     device.AppVersion,
		LastRegistered: device.LastRegistered.Format(time.RFC3339),
		CreatedAt:      device.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      device.UpdatedAt.Format(time.RFC3339),
	}
}
