package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Maximum number of search results allowed per page.
const maxSearchLimit = 50

// Default page size for search results.
const defaultSearchLimit = 20

// SearchHandler handles message search requests.
type SearchHandler struct {
	pool *pgxpool.Pool
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(pool *pgxpool.Pool) *SearchHandler {
	return &SearchHandler{pool: pool}
}

// SearchResult is a single search hit returned by the API.
type SearchResult struct {
	ID          string `json:"id"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	SenderType  string `json:"sender_type"`
	SenderID    string `json:"sender_id"`
	SenderName  string `json:"sender_name"`
	Content     string `json:"content"`
	Highlight   string `json:"highlight,omitempty"`
	ContentType string `json:"content_type"`
	CreatedAt   string `json:"created_at"`
}

// SearchResponse is the paginated search response envelope.
type SearchResponse struct {
	Results     []SearchResult `json:"results"`
	NextCursor  *string        `json:"next_cursor"`
	HasMore     bool           `json:"has_more"`
	TotalApprox int            `json:"total_approx"`
}

// Search handles GET /api/v1/search?q={query}&channel_id={optional}&limit={20}&before={cursor}
//
// The search uses PostgreSQL full-text search via plainto_tsquery (safe parsing of
// arbitrary user input including special characters). Results are permission-filtered
// to only include messages from channels the requesting user is a member of, or DMs
// they participate in. Each result includes an HTML-highlighted snippet via ts_headline.
// Cursor pagination uses created_at timestamps (RFC 3339).
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// --- Parse and validate query parameters ---

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "search query 'q' is required")
		return
	}
	if len(q) > 500 {
		writeError(w, http.StatusBadRequest, "search query exceeds maximum length of 500 characters")
		return
	}

	// Parse optional channel_id filter
	var channelID *uuid.UUID
	if raw := r.URL.Query().Get("channel_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel_id: must be a valid UUID")
			return
		}
		channelID = &id
	}

	// Parse limit with clamping (default 20, max 50)
	limit := defaultSearchLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			if parsed < 1 {
				limit = 1
			} else if parsed > maxSearchLimit {
				limit = maxSearchLimit
			} else {
				limit = parsed
			}
		}
	}

	// Parse optional cursor (RFC 3339 timestamp of the last result's created_at)
	var before *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid before cursor: must be an RFC 3339 timestamp")
			return
		}
		before = &t
	}

	// Build cursor arg for SQL — pass nil so the IS NULL check triggers.
	var cursorArg any
	if before != nil {
		cursorArg = *before
	} else {
		cursorArg = nil
	}

	// --- Execute search with permission filtering ---
	//
	// We use plainto_tsquery('simple', $1) to safely parse user input:
	//   - Handles special characters (quotes, dashes, parens) by treating them
	//     as whitespace/delimiters, avoiding tsquery syntax errors.
	//   - The 'simple' config avoids language-specific stemming/stopwords,
	//     preserving exact token matching appropriate for a chat tool.
	//
	// Performance note: ts_headline is applied only to the final limited result set
	// (LIMIT clause is evaluated before SELECT expressions in the outer query,
	// but ts_headline in the SELECT list executes per returned row). Since the
	// GIN index on search_vector efficiently filters down to limit rows, the
	// ts_headline overhead is bounded.

	query := `WITH user_channel_ids AS (
		    SELECT channel_id FROM channel_members
		    WHERE member_type = 'user' AND member_id = $5
		    UNION
		    SELECT channel_id FROM dm_members
		    WHERE member_type = 'user' AND member_id = $5
		),
		search_query AS (
		    SELECT plainto_tsquery('simple', $1) AS q
		)
		SELECT m.id, m.channel_id, c.name AS channel_name,
		       m.sender_type, m.sender_id,
		       COALESCE(u.display_name, a.name, '') AS sender_name,
		       m.content, m.content_type, m.created_at,
		       ts_headline('simple', m.content, sq.q,
		           'StartSel=<mark>, StopSel=</mark>, MaxWords=30, MinWords=10, ShortWord=3, MaxFragments=2, FragmentDelimiter=...') AS highlight
		FROM messages m
		JOIN channels c ON m.channel_id = c.id
		JOIN user_channel_ids uc ON m.channel_id = uc.channel_id
		CROSS JOIN search_query sq
		LEFT JOIN users u ON m.sender_type = 'user' AND m.sender_id = u.id
		LEFT JOIN agents a ON m.sender_type = 'agent' AND m.sender_id = a.id
		WHERE m.is_deleted = false
		  AND m.search_vector @@ sq.q
		  AND ($2::uuid IS NULL OR m.channel_id = $2)
		  AND ($3::timestamptz IS NULL OR m.created_at < $3)
		ORDER BY ts_rank(m.search_vector, sq.q) DESC, m.created_at DESC, m.id DESC
		LIMIT $4`

	// Fetch limit+1 rows so we can detect has_more.
	rows, err := h.pool.Query(r.Context(), query, q, channelID, cursorArg, limit+1, userID)
	if err != nil {
		slog.Error("search query failed", "error", err, "user_id", userID, "q", q)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	results := make([]SearchResult, 0, limit)
	for rows.Next() {
		var sr SearchResult
		var createdAt time.Time
		var highlight *string

		if err := rows.Scan(
			&sr.ID, &sr.ChannelID, &sr.ChannelName,
			&sr.SenderType, &sr.SenderID,
			&sr.SenderName, &sr.Content, &sr.ContentType, &createdAt,
			&highlight,
		); err != nil {
			slog.Error("failed to scan search result row", "error", err)
			continue
		}

		sr.CreatedAt = createdAt.Format(time.RFC3339)
		if highlight != nil && *highlight != "" {
			sr.Highlight = *highlight
		}

		results = append(results, sr)
	}
	if err := rows.Err(); err != nil {
		slog.Error("search result iteration error", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	// Return empty array, not null.
	if results == nil {
		results = make([]SearchResult, 0)
	}

	// Determine has_more and build next_cursor.
	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	var nextCursor *string
	if hasMore && len(results) > 0 {
		last := results[len(results)-1]
		nextCursor = &last.CreatedAt
	}

	// Approximate total count for UX.
	totalApprox := h.countApprox(r, q, channelID, userID)

	resp := SearchResponse{
		Results:     results,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
		TotalApprox: totalApprox,
	}

	writeJSON(w, http.StatusOK, resp)
}

// countApprox returns an approximate count of matching messages for the given
// search query and filters. It runs a separate COUNT(*) with the same permission
// and deleted filters, but without cursor/limit.
// Returns -1 if the count query fails.
func (h *SearchHandler) countApprox(r *http.Request, q string, channelID *uuid.UUID, userID string) int {
	countQuery := `WITH user_channel_ids AS (
		    SELECT channel_id FROM channel_members
		    WHERE member_type = 'user' AND member_id = $3
		    UNION
		    SELECT channel_id FROM dm_members
		    WHERE member_type = 'user' AND member_id = $3
		)
		SELECT COUNT(*)
		FROM messages m
		JOIN user_channel_ids uc ON m.channel_id = uc.channel_id
		WHERE m.is_deleted = false
		  AND m.search_vector @@ plainto_tsquery('simple', $1)
		  AND ($2::uuid IS NULL OR m.channel_id = $2)`

	var count int
	err := h.pool.QueryRow(r.Context(), countQuery, q, channelID, userID).Scan(&count)
	if err != nil {
		slog.Warn("failed to get search count approximation", "error", err)
		return -1
	}
	return count
}
