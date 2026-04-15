package management

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/trafficlog"
)

// trafficLogRow is the full, flat representation of a traffic_logs row including DB fields.
type trafficLogRow struct {
	ID          int64   `json:"id"`
	RequestID   string  `json:"request_id"`
	Provider    string  `json:"provider"`
	Endpoint    string  `json:"endpoint"`
	Model       string  `json:"model"`
	Method      string  `json:"method"`
	StatusCode  int     `json:"status_code"`
	DurationMs  int64   `json:"duration_ms"`
	IsStreaming bool    `json:"is_streaming"`
	ConfigHash  string  `json:"config_hash"`
	RawRequest  string  `json:"raw_request"`
	ProcRequest string  `json:"proc_request"`
	Response    string  `json:"response"`
	SSEPayload  string  `json:"sse_payload"`
	ErrorInfo   string  `json:"error_info"`
	CreatedAt   string  `json:"created_at"`
}

// ensureDB returns the DB connection if traffic logging is enabled, else responds with 503 and returns nil.
func ensureDB(c *gin.Context) *sql.DB {
	tl := trafficlog.GetGlobalLogger()
	if tl == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Traffic log is not enabled"})
		return nil
	}
	db := tl.GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return nil
	}
	return db
}

// scanTrafficRow scans a single DB row into a trafficLogRow.
// Expected column order: id, request_id, provider, endpoint, model, method,
// status_code, duration_ms, is_streaming, config_hash, raw_request, proc_request,
// response, sse_payload, error_info, created_at
func scanTrafficRow(rows *sql.Rows) (trafficLogRow, error) {
	var r trafficLogRow
	var isStreaming int
	var rawReq, procReq, resp, sse, errInfo sql.NullString
	err := rows.Scan(
		&r.ID, &r.RequestID, &r.Provider, &r.Endpoint, &r.Model, &r.Method,
		&r.StatusCode, &r.DurationMs, &isStreaming, &r.ConfigHash,
		&rawReq, &procReq, &resp, &sse, &errInfo, &r.CreatedAt,
	)
	if err != nil {
		return r, err
	}
	r.IsStreaming = (isStreaming == 1)
	r.RawRequest = rawReq.String
	r.ProcRequest = procReq.String
	r.Response = resp.String
	r.SSEPayload = sse.String
	r.ErrorInfo = errInfo.String
	return r, nil
}

// buildWhere builds the WHERE clause and args from query params.
func buildWhere(c *gin.Context) (string, []interface{}) {
	qBase := "FROM traffic_logs WHERE 1=1"
	var args []interface{}

	if provider := c.Query("provider"); provider != "" {
		qBase += " AND provider = ?"
		args = append(args, provider)
	}
	if model := c.Query("model"); model != "" {
		qBase += " AND model = ?"
		args = append(args, model)
	}
	if isStreamingStr := c.Query("is_streaming"); isStreamingStr != "" {
		if isStreamingStr == "1" || isStreamingStr == "true" {
			qBase += " AND is_streaming = 1"
		} else if isStreamingStr == "0" || isStreamingStr == "false" {
			qBase += " AND is_streaming = 0"
		}
	}
	if afterDate := c.Query("after"); afterDate != "" {
		qBase += " AND created_at >= ?"
		args = append(args, afterDate)
	}
	if beforeDate := c.Query("before"); beforeDate != "" {
		qBase += " AND created_at <= ?"
		args = append(args, beforeDate)
	}
	return qBase, args
}

// GetTrafficLogs handles GET /v0/management/traffic-logs
// Supports: page, limit, sort (ASC|DESC), provider, model, is_streaming, after, before
func (h *Handler) GetTrafficLogs(c *gin.Context) {
	db := ensureDB(c)
	if db == nil {
		return
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 1000 {
		limit = 50
	}
	offset := (page - 1) * limit

	sortOrder := strings.ToUpper(c.DefaultQuery("sort", "DESC"))
	if sortOrder != "ASC" {
		sortOrder = "DESC"
	}

	qBase, args := buildWhere(c)

	// Count — single query only
	var total int
	if err := db.QueryRow("SELECT COUNT(*) "+qBase, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count records", "details": err.Error()})
		return
	}
	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// Fetch — single query, no double-query
	fetchSQL := fmt.Sprintf(
		"SELECT id, request_id, provider, endpoint, model, method, status_code, duration_ms, is_streaming, config_hash, raw_request, proc_request, response, sse_payload, error_info, created_at %s ORDER BY created_at %s LIMIT ? OFFSET ?",
		qBase, sortOrder,
	)
	rows, err := db.Query(fetchSQL, append(args, limit, offset)...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch records", "details": err.Error()})
		return
	}
	defer rows.Close()

	logs := make([]trafficLogRow, 0)
	for rows.Next() {
		r, err := scanTrafficRow(rows)
		if err != nil {
			continue
		}
		logs = append(logs, r)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Row iteration error", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs": logs,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// GetTrafficLogDetail handles GET /v0/management/traffic-logs/detail/:id
func (h *Handler) GetTrafficLogDetail(c *gin.Context) {
	db := ensureDB(c)
	if db == nil {
		return
	}

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid ID format: %q", idStr)})
		return
	}

	const query = `
		SELECT id, request_id, provider, endpoint, model, method, status_code, duration_ms,
		       is_streaming, config_hash, raw_request, proc_request, response, sse_payload,
		       error_info, created_at
		FROM traffic_logs WHERE id = ?
	`

	var r trafficLogRow
	var isStreaming int
	var rawReq, procReq, resp, sse, errInfo sql.NullString

	err = db.QueryRow(query, id).Scan(
		&r.ID, &r.RequestID, &r.Provider, &r.Endpoint, &r.Model, &r.Method,
		&r.StatusCode, &r.DurationMs, &isStreaming, &r.ConfigHash,
		&rawReq, &procReq, &resp, &sse, &errInfo, &r.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Record not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch record", "details": err.Error()})
		}
		return
	}

	r.IsStreaming = (isStreaming == 1)
	r.RawRequest = rawReq.String
	r.ProcRequest = procReq.String
	r.Response = resp.String
	r.SSEPayload = sse.String
	r.ErrorInfo = errInfo.String

	c.JSON(http.StatusOK, r)
}

// DeleteTrafficLogs handles DELETE /v0/management/traffic-logs
// Wipes all logs and reclaims WAL space.
func (h *Handler) DeleteTrafficLogs(c *gin.Context) {
	db := ensureDB(c)
	if db == nil {
		return
	}

	if _, err := db.Exec("DELETE FROM traffic_logs"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete logs", "details": err.Error()})
		return
	}
	// Reclaim WAL and free pages
	db.Exec("PRAGMA wal_checkpoint(TRUNCATE)") //nolint:errcheck
	db.Exec("PRAGMA incremental_vacuum")       //nolint:errcheck

	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "All traffic logs wiped successfully"})
}

// GetTrafficLogStats handles GET /v0/management/traffic-logs/stats
func (h *Handler) GetTrafficLogStats(c *gin.Context) {
	db := ensureDB(c)
	if db == nil {
		return
	}

	var totalRecords int
	db.QueryRow("SELECT COUNT(*) FROM traffic_logs").Scan(&totalRecords) //nolint:errcheck

	var oldest, newest sql.NullString
	db.QueryRow("SELECT MIN(created_at), MAX(created_at) FROM traffic_logs").Scan(&oldest, &newest) //nolint:errcheck

	// Provider breakdown
	providers := make(map[string]int)
	if rows, err := db.Query("SELECT COALESCE(NULLIF(provider,''), 'unknown'), COUNT(*) FROM traffic_logs GROUP BY provider"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var p string
			var count int
			if rows.Scan(&p, &count) == nil {
				providers[p] = count
			}
		}
	}

	// Model breakdown
	models := make(map[string]int)
	if rows, err := db.Query("SELECT COALESCE(NULLIF(model,''), 'unknown'), COUNT(*) FROM traffic_logs GROUP BY model"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var m string
			var count int
			if rows.Scan(&m, &count) == nil {
				models[m] = count
			}
		}
	}

	// Status code breakdown: 2xx, 4xx, 5xx
	statusBreakdown := map[string]int{"2xx": 0, "4xx": 0, "5xx": 0, "other": 0}
	if rows, err := db.Query("SELECT status_code, COUNT(*) FROM traffic_logs GROUP BY status_code"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var sc, count int
			if rows.Scan(&sc, &count) == nil {
				switch {
				case sc >= 200 && sc < 300:
					statusBreakdown["2xx"] += count
				case sc >= 400 && sc < 500:
					statusBreakdown["4xx"] += count
				case sc >= 500:
					statusBreakdown["5xx"] += count
				default:
					statusBreakdown["other"] += count
				}
			}
		}
	}

	// Average duration
	var avgDuration sql.NullFloat64
	db.QueryRow("SELECT AVG(duration_ms) FROM traffic_logs").Scan(&avgDuration) //nolint:errcheck

	// DB size via PRAGMA
	var dbSizeBytes int64
	var dbSizeHuman string
	var pages, pageSize int64
	if err := db.QueryRow("PRAGMA page_count").Scan(&pages); err == nil {
		if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err == nil {
			dbSizeBytes = pages * pageSize
			dbSizeHuman = formatBytes(dbSizeBytes)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_records":    totalRecords,
		"db_size_bytes":    dbSizeBytes,
		"db_size_human":    dbSizeHuman,
		"providers":        providers,
		"models":           models,
		"status_breakdown": statusBreakdown,
		"avg_duration_ms":  avgDuration.Float64,
		"oldest_record":    oldest.String,
		"newest_record":    newest.String,
	})
}

// GetTrafficLogConfigSnapshot handles GET /v0/management/traffic-logs/config-snapshot/:hash
func (h *Handler) GetTrafficLogConfigSnapshot(c *gin.Context) {
	db := ensureDB(c)
	if db == nil {
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Hash parameter is required"})
		return
	}

	var content, createdAt string
	err := db.QueryRow("SELECT content, created_at FROM config_snapshots WHERE hash = ?", hash).Scan(&content, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Config snapshot not found for hash: " + hash})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch snapshot", "details": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"hash":       hash,
		"content":    content,
		"created_at": createdAt,
	})
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
