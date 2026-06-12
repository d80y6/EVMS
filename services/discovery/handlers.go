package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/dam-vms/dam/pkg/onvif"
	"github.com/google/uuid"
)

type ScanHandler struct {
	orchestrator *ScanOrchestrator
	store        *ResultStore
	logger       *slog.Logger
}

func NewScanHandler(orchestrator *ScanOrchestrator, store *ResultStore, logger *slog.Logger) *ScanHandler {
	return &ScanHandler{
		orchestrator: orchestrator,
		store:        store,
		logger:       logger,
	}
}

func (h *ScanHandler) handleCreateScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SiteID  string   `json:"site_id"`
		Methods []string `json:"methods"`
		Subnets []string `json:"subnets"`
		Ports   []int    `json:"ports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var siteID *uuid.UUID
	if req.SiteID != "" {
		parsed, err := uuid.Parse(req.SiteID)
		if err != nil {
			jsonError(w, "invalid site_id", http.StatusBadRequest)
			return
		}
		siteID = &parsed
	}
	if len(req.Methods) == 0 {
		req.Methods = []string{"ws-discovery"}
	}
	if len(req.Ports) == 0 {
		req.Ports = []int{80, 554, 8080}
	}
	var userID *uuid.UUID
	if uid, err := common.GetUserIDFromContext(r.Context()); err == nil {
		userID = &uid
	}

	scan, err := h.orchestrator.StartScan(r.Context(), ScanRequest{
		SiteID:    siteID,
		Methods:   req.Methods,
		Subnets:   req.Subnets,
		Ports:     req.Ports,
		CreatedBy: userID,
	})
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, scan)
}

func (h *ScanHandler) handleListScans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var siteID *uuid.UUID
	if sid := r.URL.Query().Get("site_id"); sid != "" {
		parsed, err := uuid.Parse(sid)
		if err == nil {
			siteID = &parsed
		}
	}

	scans, total, err := h.store.GetScans(r.Context(), siteID, page, perPage)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if scans == nil {
		scans = []ScanRecord{}
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"scans":    scans,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *ScanHandler) handleGetScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scanID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid scan id", http.StatusBadRequest)
		return
	}
	scan, err := h.store.GetScan(r.Context(), scanID)
	if err != nil {
		jsonError(w, "scan not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, http.StatusOK, scan)
}

func (h *ScanHandler) handleCancelScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scanID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid scan id", http.StatusBadRequest)
		return
	}
	if err := h.orchestrator.CancelScan(r.Context(), scanID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *ScanHandler) handleGetResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scanID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid scan id", http.StatusBadRequest)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	query := r.URL.Query().Get("query")

	results, total, err := h.store.GetResults(r.Context(), scanID, page, perPage, query)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []ResultRecord{}
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"results":  results,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *ScanHandler) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scanID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid scan id", http.StatusBadRequest)
		return
	}
	_ = scanID
	var req struct {
		ResultIDs   []string `json:"result_ids"`
		Credentials []struct {
			ResultID string `json:"result_id"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var resultUUIDs []uuid.UUID
	for _, id := range req.ResultIDs {
		uid, err := uuid.Parse(id)
		if err != nil {
			jsonError(w, "invalid result_id: "+id, http.StatusBadRequest)
			return
		}
		resultUUIDs = append(resultUUIDs, uid)
	}
	if err := h.store.MarkImported(r.Context(), resultUUIDs); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"imported": len(resultUUIDs),
		"failed":   []interface{}{},
	})
}

func (h *ScanHandler) handleTestCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		IP       string `json:"ip"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	deviceURL := "http://" + req.IP + ":" + strconv.Itoa(req.Port) + "/onvif/device_service"
	client := onvif.NewSOAPClient(5*time.Second, &onvif.Credentials{
		Username: req.Username,
		Password: req.Password,
	})
	_, err := onvif.GetDeviceInformation(r.Context(), client, deviceURL)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
