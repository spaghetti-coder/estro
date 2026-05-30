// Package handler provides HTTP request handlers for the Estro web UI.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/spaghetti-coder/estro/internal/auth"
	"github.com/spaghetti-coder/estro/internal/config"
	"github.com/spaghetti-coder/estro/internal/exec"
	"github.com/spaghetti-coder/estro/internal/job"
)

const (
	jobTTL             = 10 * time.Minute
	loginRateLimitRate = 1.0 / 90.0 // ~1 request per 90s
)

// Handler holds shared dependencies for all HTTP route handlers.
type Handler struct {
	cfg          *config.Config
	jobs         *job.Store
	sessionStore sessions.Store
	services     []config.FlatService
	issues       []string
	degraded     bool
	// cmdCtx is the application-lifecycle context, cancelled on server shutdown
	// (not request-scoped); it bounds async command execution.
	cmdCtx context.Context
}

// NewHandler creates a Handler from a config LoadResult. In degraded mode it
// serves no services and exposes the issue list instead.
func NewHandler(res *config.LoadResult, jobStore *job.Store, sessionStore sessions.Store, cmdCtx context.Context) *Handler {
	h := &Handler{
		cfg:          res.Config,
		jobs:         jobStore,
		sessionStore: sessionStore,
		issues:       res.IssueStrings(),
		degraded:     !res.Healthy(),
		cmdCtx:       cmdCtx,
	}
	if !h.degraded {
		h.services = res.Config.Flatten()
	}
	return h
}

// healthzResponse is the JSON body of the /healthz endpoint.
type healthzResponse struct {
	Status string   `json:"status"`
	Issues []string `json:"issues,omitempty"`
}

// errJSON writes a JSON error response with the given status code and message.
func errJSON(c *echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

// tooManyLogins is the shared response for login rate-limit rejections.
func tooManyLogins(c *echo.Context) error {
	return errJSON(c, http.StatusTooManyRequests, "Too many login attempts")
}

// RegisterRoutes registers all HTTP routes on the given Echo instance.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	loginLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Skipper: middleware.DefaultSkipper,
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      loginRateLimitRate,
				Burst:     10,
				ExpiresIn: 15 * time.Minute,
			},
		),
		IdentifierExtractor: func(c *echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c *echo.Context, err error) error {
			return tooManyLogins(c)
		},
		DenyHandler: func(c *echo.Context, identifier string, err error) error {
			return tooManyLogins(c)
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
	if h.degraded {
		return c.JSON(http.StatusServiceUnavailable, healthzResponse{Status: "error", Issues: h.issues})
	}
	return c.JSON(http.StatusOK, healthzResponse{Status: "ok"})
}

func (h *Handler) getConfig(c *echo.Context) error {
	resp := h.cfg.GetConfigResponse()
	resp.Degraded = h.degraded
	resp.Issues = h.issues
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) listServices(c *echo.Context) error {
	if h.degraded {
		return c.JSON(http.StatusOK, []config.SerializedService{})
	}
	username := auth.GetSessionUser(h.sessionStore, c.Request())
	result := []config.SerializedService{}
	for i, svc := range h.services {
		if svc.IsHidden(username) {
			continue
		}
		result = append(result, svc.Serialize(i, username))
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) getMe(c *echo.Context) error {
	username := auth.GetSessionUser(h.sessionStore, c.Request())
	if username == "" {
		return c.JSON(http.StatusOK, nil)
	}
	return c.JSON(http.StatusOK, map[string]string{
		"username": username,
	})
}

func (h *Handler) login(c *echo.Context) error {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"rememberMe"`
	}
	if err := c.Bind(&body); err != nil {
		return errJSON(c, http.StatusBadRequest, "Invalid request")
	}
	if auth.Authenticate(h.cfg.Users, body.Username, body.Password) == nil {
		return errJSON(c, http.StatusUnauthorized, "Invalid username or password")
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
		return errJSON(c, http.StatusNotFound, "Unknown service")
	}

	svc := h.services[svcIndex]

	if !svc.Enabled {
		return errJSON(c, http.StatusForbidden, "Service disabled")
	}

	username := auth.GetSessionUser(h.sessionStore, c.Request())
	if svc.IsHidden(username) {
		return errJSON(c, http.StatusNotFound, "Unknown service")
	}
	if !svc.IsAccessible(username) {
		return errJSON(c, http.StatusForbidden, "Forbidden")
	}

	sshOpts := strings.Join(svc.RemoteSSHOpts, " ")
	cmd, err := exec.BuildCmd(svc.Command, svc.Remote, sshOpts)
	if err != nil {
		return errJSON(c, http.StatusBadRequest, err.Error())
	}

	jobID, err := job.GenerateID()
	if err != nil {
		return errJSON(c, http.StatusInternalServerError, "Failed to generate job ID")
	}
	h.jobs.Set(jobID, &job.Job{Status: job.StatusRunning, Title: svc.Title})

	if err := c.JSON(http.StatusAccepted, map[string]string{"jobId": jobID}); err != nil {
		h.jobs.Delete(jobID)
		return err
	}

	go h.executeAsync(jobID, svc, cmd)

	return nil
}

func (h *Handler) executeAsync(jobID string, svc config.FlatService, cmd string) {
	timeout := time.Duration(svc.Timeout) * time.Second
	stdout, stderr, cmdErr := exec.RunCommand(h.cmdCtx, cmd, timeout)
	status := job.StatusDone
	if cmdErr != nil {
		status = job.StatusError
		// Surface the command's own stderr when it produced any; otherwise fall
		// back to the Go execution error (e.g. "exit status 1") so the failure
		// is never reported with an empty message.
		if stderr == "" {
			stderr = cmdErr.Error()
		}
	}
	h.jobs.Set(jobID, &job.Job{
		Status: status,
		Title:  svc.Title,
		Stdout: stdout,
		Stderr: stderr,
	})
	h.jobs.ScheduleCleanup(jobID, jobTTL)
}

func (h *Handler) getJob(c *echo.Context) error {
	id := c.Param("id")
	j, ok := h.jobs.Get(id)
	if !ok {
		return errJSON(c, http.StatusNotFound, "Unknown job")
	}
	return c.JSON(http.StatusOK, j)
}
