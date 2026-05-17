// Package handler provides HTTP request handlers for the Estro web UI.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/spaghetti-coder/estro/internal/auth"
	"github.com/spaghetti-coder/estro/internal/config"
	"github.com/spaghetti-coder/estro/internal/exec"
	"github.com/spaghetti-coder/estro/internal/job"
)

const jobTTL = 10 * time.Minute

// Handler holds shared dependencies for all HTTP route handlers.
type Handler struct {
	cfg           *config.Config
	jobs          *job.Store
	sessionStore  sessions.Store
	sessionSecret []byte
	services      []config.FlatService
	cmdCtx        context.Context
}

// NewHandler creates a Handler with the provided dependencies.
func NewHandler(cfg *config.Config, jobStore *job.Store, sessionStore sessions.Store, sessionSecret []byte, cmdCtx context.Context) *Handler {
	return &Handler{
		cfg:           cfg,
		jobs:          jobStore,
		sessionStore:  sessionStore,
		sessionSecret: sessionSecret,
		services:      cfg.Flatten(),
		cmdCtx:        cmdCtx,
	}
}

// RegisterRoutes registers all HTTP routes on the given Echo instance.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	loginLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Skipper: middleware.DefaultSkipper,
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      0.011111111111111112,
				Burst:     10,
				ExpiresIn: 15 * time.Minute,
			},
		),
		IdentifierExtractor: func(c *echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c *echo.Context, err error) error {
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Too many login attempts"})
		},
		DenyHandler: func(c *echo.Context, identifier string, err error) error {
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Too many login attempts"})
		},
	})

	e.GET("/healthz", h.healthz)
	e.GET("/config", h.getConfig)
	e.GET("/services", h.listServices)
	e.GET("/me", h.getMe)
	e.POST("/login", h.login, loginLimiter)
	e.POST("/logout", h.logout)
	e.POST("/run/:svc", h.runService)
	e.GET("/jobs/:id", h.getJob)
}

func (h *Handler) healthz(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) getConfig(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.cfg.GetConfigResponse())
}

func (h *Handler) listServices(c *echo.Context) error {
	username, _ := auth.GetSessionUser(h.sessionStore, c.Request(), c.Response())
	var result []config.SerializedService
	for i, svc := range h.services {
		if svc.GetRestricted() && !svc.IsAccessible(username, h.cfg.Users) {
			continue
		}
		result = append(result, svc.Serialize(i, username, h.cfg.Users))
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) getMe(c *echo.Context) error {
	username, _ := auth.GetSessionUser(h.sessionStore, c.Request(), c.Response())
	if username == "" {
		return c.JSON(http.StatusOK, nil)
	}
	user, ok := h.cfg.Users[username]
	groups := []string{}
	if ok && user.Groups != nil {
		groups = user.Groups
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"username": username,
		"groups":   groups,
	})
}

func (h *Handler) login(c *echo.Context) error {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"rememberMe"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}
	if body.Username == "" || body.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Username and password required"})
	}
	_, err := auth.Authenticate(h.cfg.Users, body.Username, body.Password)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid username or password"})
	}
	if err := auth.SetSessionUser(h.sessionStore, c.Request(), c.Response(), body.Username, body.RememberMe); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to save session")
	}
	return c.JSON(http.StatusOK, map[string]string{"username": body.Username})
}

func (h *Handler) logout(c *echo.Context) error {
	_ = auth.DestroySession(h.sessionStore, c.Request(), c.Response())
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) runService(c *echo.Context) error {
	svcIndex, err := strconv.Atoi(c.Param("svc"))
	if err != nil || svcIndex < 0 || svcIndex >= len(h.services) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Unknown service"})
	}

	svc := h.services[svcIndex]

	if !svc.GetEnabled() {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Service disabled"})
	}

	username, _ := auth.GetSessionUser(h.sessionStore, c.Request(), c.Response())
	if svc.GetRestricted() && !svc.IsAccessible(username, h.cfg.Users) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Unknown service"})
	}
	if !svc.IsAccessible(username, h.cfg.Users) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Forbidden"})
	}

	remote := svc.GetRemote()
	cmd, err := exec.BuildCmd(svc.Command, remote)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	jobID := job.GenerateID()
	h.jobs.Set(jobID, &job.Job{Status: "running", Title: svc.Title})

	if err := c.JSON(http.StatusAccepted, map[string]string{"jobId": jobID}); err != nil {
		h.jobs.Delete(jobID)
		return err
	}

	go h.executeAsync(jobID, svc, cmd)

	return nil
}

func (h *Handler) executeAsync(jobID string, svc config.FlatService, cmd string) {
	timeout := time.Duration(svc.GetTimeout()) * time.Second
	stdout, stderr, cmdErr := exec.RunCommand(h.cmdCtx, cmd, timeout)
	if cmdErr != nil {
		if stderr == "" {
			stderr = cmdErr.Error()
		}
		h.jobs.Set(jobID, &job.Job{
			Status: "error",
			Title:  svc.Title,
			Stdout: stdout,
			Stderr: stderr,
		})
	} else {
		h.jobs.Set(jobID, &job.Job{
			Status: "done",
			Title:  svc.Title,
			Stdout: stdout,
			Stderr: stderr,
		})
	}
	h.jobs.ScheduleCleanup(jobID, jobTTL)
}

func (h *Handler) getJob(c *echo.Context) error {
	id := c.Param("id")
	j, ok := h.jobs.Get(id)
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Unknown job"})
	}
	return c.JSON(http.StatusOK, j)
}
