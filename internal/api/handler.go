package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prajwalmahajan101/toybloom/internal/api/gen"
	"github.com/prajwalmahajan101/toybloom/internal/core/logger"
	"github.com/prajwalmahajan101/toybloom/internal/core/response"
	"github.com/prajwalmahajan101/toybloom/internal/service"
)

// Handler implements gen.ServerInterface — the operation set + signatures are
// generated from api/openapi.yaml, so a mismatch is a compile error (drift-proof).
type Handler struct {
	svc service.FilterService
}

func NewHandler(svc service.FilterService) *Handler {
	return &Handler{svc: svc}
}

// compile-time proof the handler satisfies the generated contract.
var _ gen.ServerInterface = (*Handler)(nil)

func writeSuccess(c *gin.Context, status int, message string, data any) {
	c.JSON(status, response.NewSuccess(getCorrelationID(c), message, data))
}

func writeError(c *gin.Context, status int, code, msg string) {
	c.JSON(status, response.NewError(getCorrelationID(c), msg,
		response.ErrorDetail{Code: code, Message: msg}))
}

// errorToHTTP maps a service sentinel to (status, machine code). Adding a new
// error family is a one-line change here.
func errorToHTTP(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrAlreadyExists):
		return http.StatusConflict, "ALREADY_EXISTS"
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, service.ErrInvalidArgument):
		return http.StatusBadRequest, "INVALID_ARGUMENT"
	default:
		return http.StatusInternalServerError, "INTERNAL"
	}
}

func mapError(c *gin.Context, err error) {
	status, code := errorToHTTP(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		logger.FromContext(c.Request.Context()).Error("request failed", "err", err)
		msg = "internal server error" // never leak internal error text
	}
	writeError(c, status, code, msg)
}

// ── gen.ServerInterface implementation ──────────────────────────────────────
// Requests are already schema-validated by the OapiRequestValidator middleware
// (name pattern, n>=1, 0<p<1, required fields) before these run.

func (h *Handler) CreateFilter(c *gin.Context, _ gen.CreateFilterParams) {
	var req gen.CreateFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	info, err := h.svc.Create(c.Request.Context(), req.Name, uint64(req.N), req.P)
	if err != nil {
		mapError(c, err)
		return
	}
	writeSuccess(c, http.StatusCreated, "filter created", toFilterInfo(info))
}

func (h *Handler) AddItem(c *gin.Context, name gen.FilterName, _ gen.AddItemParams) {
	var req gen.AddItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	if err := h.svc.Add(c.Request.Context(), name, []byte(req.Value)); err != nil {
		mapError(c, err)
		return
	}
	writeSuccess(c, http.StatusOK, "item added", gen.AddedResponse{Added: true})
}

func (h *Handler) CheckItem(c *gin.Context, name gen.FilterName, value string, _ gen.CheckItemParams) {
	exists, err := h.svc.Exists(c.Request.Context(), name, []byte(value))
	if err != nil {
		mapError(c, err)
		return
	}
	writeSuccess(c, http.StatusOK, "ok", gen.ExistsResponse{Exists: exists})
}

func (h *Handler) GetFilter(c *gin.Context, name gen.FilterName, _ gen.GetFilterParams) {
	stats, err := h.svc.Stats(c.Request.Context(), name)
	if err != nil {
		mapError(c, err)
		return
	}
	writeSuccess(c, http.StatusOK, "ok", toFilterStats(stats))
}

func (h *Handler) DeleteFilter(c *gin.Context, name gen.FilterName, _ gen.DeleteFilterParams) {
	if err := h.svc.Delete(c.Request.Context(), name); err != nil {
		mapError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── service DTO → generated wire model (the codegen boundary) ────────────────

func toFilterInfo(f service.FilterInfo) gen.FilterInfo {
	return gen.FilterInfo{Name: f.Name, Stages: f.Stages, M: int64(f.M), K: f.K}
}

func toFilterStats(s service.FilterStats) gen.FilterStats {
	stages := make([]gen.StageInfo, len(s.Stages))
	for i, st := range s.Stages {
		stages[i] = gen.StageInfo{
			Index:    st.Index,
			M:        int64(st.M),
			K:        st.K,
			Capacity: int64(st.Capacity),
			Fill:     int64(st.Fill),
		}
	}
	return gen.FilterStats{
		Name:       s.Name,
		N:          int64(s.N),
		P:          s.P,
		R:          s.R,
		S:          s.S,
		StageCount: s.StageCount,
		Stages:     stages,
	}
}
