package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/realtime"
)

// KnowledgeEntry represents a single knowledge document with vector embedding.
type KnowledgeEntry struct {
	ID            string    `json:"id"`
	ChannelID     string    `json:"channel_id"`
	AuthorAgentID string    `json:"author_agent_id"`
	AuthorName    string    `json:"author_name,omitempty"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Tags          []string  `json:"tags"`
	Source        string    `json:"source"`
	SourceRef     string    `json:"source_ref,omitempty"`
	ViewCount     int       `json:"view_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Similarity    float64   `json:"similarity,omitempty"`
}

// CreateKnowledgeRequest is the payload for creating a knowledge entry.
type CreateKnowledgeRequest struct {
	ChannelID     string   `json:"channel_id"`
	AuthorAgentID string   `json:"author_agent_id"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags,omitempty"`
	Source        string   `json:"source,omitempty"`
}

// UpdateKnowledgeRequest is the payload for updating a knowledge entry.
type UpdateKnowledgeRequest struct {
	Title   *string  `json:"title,omitempty"`
	Content *string  `json:"content,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// KnowledgeKnowledgeService handles CRUD and semantic search for knowledge entries.
type KnowledgeService struct {
	pool     *pgxpool.Pool
	embedSvc *EmbeddingService
	hub      realtime.Broadcaster
}

// NewKnowledgeService creates a new KnowledgeService.
func NewKnowledgeService(pool *pgxpool.Pool, embedSvc *EmbeddingService) *KnowledgeService {
	return &KnowledgeService{pool: pool, embedSvc: embedSvc}
}

// SetHub injects a WebSocket broadcaster for real-time events.
func (s *KnowledgeService) SetHub(hub realtime.Broadcaster) {
	s.hub = hub
}

// Create inserts a new knowledge entry and asynchronously generates its embedding.
func (s *KnowledgeService) Create(ctx context.Context, authorAgentID string, req CreateKnowledgeRequest) (*KnowledgeEntry, error) {
	if req.Title == "" {
		return nil, errors.New("title is required")
	}
	if req.Content == "" {
		return nil, errors.New("content is required")
	}
	if req.Source == "" {
		req.Source = "manual"
	}

	id := uuid.New().String()
	now := time.Now()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO knowledge (id, channel_id, author_agent_id, title, content, tags, source, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, req.ChannelID, authorAgentID, req.Title, req.Content, req.Tags, req.Source, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert knowledge: %w", err)
	}

	// Resolve author name.
	var authorName string
	_ = s.pool.QueryRow(ctx, `SELECT name FROM agents WHERE id = $1`, authorAgentID).Scan(&authorName)

	entry := &KnowledgeEntry{
		ID:            id,
		ChannelID:     req.ChannelID,
		AuthorAgentID: authorAgentID,
		AuthorName:    authorName,
		Title:         req.Title,
		Content:       req.Content,
		Tags:          req.Tags,
		Source:        req.Source,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Asynchronously generate embedding.
	go func() {
		embedding, err := s.embedSvc.GenerateEmbedding(context.Background(), req.Title+"\n"+req.Content)
		if err != nil {
			slog.Warn("embedding generation failed", "knowledge_id", id, "error", err)
			return
		}
		vecStr := vectorToString(embedding)
		_, err = s.pool.Exec(context.Background(),
			`UPDATE knowledge SET embedding = $1::vector WHERE id = $2`, vecStr, id)
		if err != nil {
			slog.Warn("failed to store embedding", "knowledge_id", id, "error", err)
		}
	}()

	slog.Info("knowledge entry created", "id", id, "channel_id", req.ChannelID, "title", req.Title)

	// Broadcast knowledge_created event.
	if s.hub != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":    "knowledge_created",
			"payload": entry,
		})
		s.hub.BroadcastToChannel(req.ChannelID, payload)
	}

	return entry, nil
}

// Search performs semantic search over knowledge entries.
// It first generates a query embedding, then does a cosine similarity search.
// Falls back to full-text search if embedding generation fails or no embedding results.
func (s *KnowledgeService) Search(ctx context.Context, channelID, query string, topK int) ([]KnowledgeEntry, error) {
	if topK <= 0 {
		topK = 5
	}

	var results []KnowledgeEntry

	// Try semantic search if we can generate a query embedding.
	embedding, embErr := s.embedSvc.GenerateEmbedding(ctx, query)
	if embErr == nil && embedding != nil {
		vecStr := vectorToString(embedding)
		// Cross-channel search when channelID is empty.
		semanticQuery := `SELECT k.id, k.title, k.content, k.tags, k.source, COALESCE(k.source_ref, ''), k.view_count,
			        k.author_agent_id, COALESCE(a.name, '') AS author_name,
			        k.created_at, k.updated_at,
			        1 - (k.embedding <=> $1::vector) AS similarity
			 FROM knowledge k
			 LEFT JOIN agents a ON k.author_agent_id = a.id
			 WHERE k.embedding IS NOT NULL
			   AND 1 - (k.embedding <=> $1::vector) > 0.7`
		var (
			rows pgx.Rows
			err  error
		)
		if channelID != "" {
			semanticQuery += ` AND k.channel_id = $2 ORDER BY k.embedding <=> $1::vector LIMIT $3`
			rows, err = s.pool.Query(ctx, semanticQuery, vecStr, channelID, topK)
		} else {
			semanticQuery += ` ORDER BY k.embedding <=> $1::vector LIMIT $2`
			rows, err = s.pool.Query(ctx, semanticQuery, vecStr, topK)
		}
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var e KnowledgeEntry
				if err := rows.Scan(&e.ID, &e.Title, &e.Content, &e.Tags, &e.Source, &e.SourceRef,
					&e.ViewCount, &e.AuthorAgentID, &e.AuthorName, &e.CreatedAt, &e.UpdatedAt, &e.Similarity); err != nil {
					return nil, fmt.Errorf("scan semantic result: %w", err)
				}
				e.ChannelID = channelID
				results = append(results, e)
			}
			if err := rows.Err(); err != nil {
				return nil, fmt.Errorf("rows error: %w", err)
			}
		} else {
			slog.Warn("semantic search query failed, falling back to FTS", "error", err)
		}
	}

	// If semantic search returned results, return them.
	if len(results) > 0 {
		return results, nil
	}

	// Full-text search fallback.
	ftsQuery := `SELECT k.id, k.title, k.content, k.tags, k.source, COALESCE(k.source_ref, ''), k.view_count,
		        k.author_agent_id, COALESCE(a.name, '') AS author_name,
		        k.created_at, k.updated_at,
		        ts_rank(to_tsvector('simple', k.title || ' ' || k.content), plainto_tsquery('simple', $1)) AS rank
		 FROM knowledge k
		 LEFT JOIN agents a ON k.author_agent_id = a.id
		 WHERE to_tsvector('simple', k.title || ' ' || k.content) @@ plainto_tsquery('simple', $1)`
	var (
		rows pgx.Rows
		err  error
	)
	if channelID != "" {
		ftsQuery += ` AND k.channel_id = $2 ORDER BY rank DESC LIMIT $3`
		rows, err = s.pool.Query(ctx, ftsQuery, query, channelID, topK)
	} else {
		ftsQuery += ` ORDER BY rank DESC LIMIT $2`
		rows, err = s.pool.Query(ctx, ftsQuery, query, topK)
	}
	if err != nil {
		return nil, fmt.Errorf("fts query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.Content, &e.Tags, &e.Source, &e.SourceRef,
			&e.ViewCount, &e.AuthorAgentID, &e.AuthorName, &e.CreatedAt, &e.UpdatedAt, &e.Similarity); err != nil {
			return nil, fmt.Errorf("scan fts result: %w", err)
		}
		e.ChannelID = channelID
		results = append(results, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	if results == nil {
		results = []KnowledgeEntry{}
	}
	return results, nil
}

// Get retrieves a single knowledge entry by ID.
func (s *KnowledgeService) Get(ctx context.Context, id string) (*KnowledgeEntry, error) {
	var e KnowledgeEntry
	err := s.pool.QueryRow(ctx,
		`SELECT k.id, k.channel_id, k.title, k.content, k.tags, k.source, COALESCE(k.source_ref, ''),
		        k.view_count, k.author_agent_id, COALESCE(a.name, '') AS author_name,
		        k.created_at, k.updated_at
		 FROM knowledge k
		 LEFT JOIN agents a ON k.author_agent_id = a.id
		 WHERE k.id = $1`, id,
	).Scan(&e.ID, &e.ChannelID, &e.Title, &e.Content, &e.Tags, &e.Source, &e.SourceRef,
		&e.ViewCount, &e.AuthorAgentID, &e.AuthorName, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("knowledge entry not found")
		}
		return nil, fmt.Errorf("get knowledge: %w", err)
	}

	// Increment view count.
	_, _ = s.pool.Exec(ctx, `UPDATE knowledge SET view_count = view_count + 1 WHERE id = $1`, id)
	return &e, nil
}

// Update modifies an existing knowledge entry.
func (s *KnowledgeService) Update(ctx context.Context, id, agentID string, req UpdateKnowledgeRequest) (*KnowledgeEntry, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing.AuthorAgentID != agentID {
		return nil, errors.New("only the author can update this entry")
	}

	now := time.Now()
	title := existing.Title
	if req.Title != nil {
		title = *req.Title
	}
	content := existing.Content
	if req.Content != nil {
		content = *req.Content
	}
	tags := existing.Tags
	if req.Tags != nil {
		tags = req.Tags
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE knowledge SET title = $1, content = $2, tags = $3, updated_at = $4 WHERE id = $5`,
		title, content, tags, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update knowledge: %w", err)
	}

	// Regenerate embedding asynchronously if content changed.
	if req.Content != nil {
		go func() {
			embedding, embErr := s.embedSvc.GenerateEmbedding(context.Background(), title+"\n"+content)
			if embErr != nil {
				slog.Warn("embedding regeneration failed", "knowledge_id", id, "error", embErr)
				return
			}
			vecStr := vectorToString(embedding)
			s.pool.Exec(context.Background(), `UPDATE knowledge SET embedding = $1::vector WHERE id = $2`, vecStr, id)
		}()
	}

	entry := *existing
	entry.Title = title
	entry.Content = content
	entry.Tags = tags
	entry.UpdatedAt = now

	slog.Info("knowledge entry updated", "id", id)

	// Broadcast knowledge_updated event.
	if s.hub != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":    "knowledge_updated",
			"payload": entry,
		})
		s.hub.BroadcastToChannel(existing.ChannelID, payload)
	}

	return &entry, nil
}

// Delete removes a knowledge entry.
func (s *KnowledgeService) Delete(ctx context.Context, id, agentID string) error {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if existing.AuthorAgentID != agentID {
		return errors.New("only the author can delete this entry")
	}

	_, err = s.pool.Exec(ctx, `DELETE FROM knowledge WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete knowledge: %w", err)
	}

	slog.Info("knowledge entry deleted", "id", id)
	return nil
}

// List returns knowledge entries for a channel, optionally filtered by tag.
func (s *KnowledgeService) List(ctx context.Context, channelID, tag string, limit, offset int) ([]KnowledgeEntry, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT k.id, k.title, k.content, k.tags, k.source, COALESCE(k.source_ref, ''),
	                  k.view_count, k.author_agent_id, COALESCE(a.name, '') AS author_name,
	                  k.created_at, k.updated_at
	           FROM knowledge k
	           LEFT JOIN agents a ON k.author_agent_id = a.id
	           WHERE k.channel_id = $1`
	args := []interface{}{channelID}
	argIdx := 2

	if tag != "" {
		query += fmt.Sprintf(" AND $%d = ANY(k.tags)", argIdx)
		args = append(args, tag)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY k.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list knowledge: %w", err)
	}
	defer rows.Close()

	var entries []KnowledgeEntry
	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.Content, &e.Tags, &e.Source, &e.SourceRef,
			&e.ViewCount, &e.AuthorAgentID, &e.AuthorName, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan knowledge: %w", err)
		}
		e.ChannelID = channelID
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []KnowledgeEntry{}
	}
	return entries, nil
}

// ImportFromDecisions reads decisions.md for a channel and imports entries
// into the knowledge table. Returns the number of imported entries.
func (s *KnowledgeService) ImportFromDecisions(ctx context.Context, channelID, authorAgentID string) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("get home dir: %w", err)
	}
	path := filepath.Join(home, ".solo", "channels", channelID, "memory", "decisions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, errors.New("decisions.md not found for this channel")
		}
		return 0, fmt.Errorf("read decisions.md: %w", err)
	}

	entries := parseDecisionBlocks(string(data))
	count := 0
	for _, entry := range entries {
		_, err := s.Create(ctx, authorAgentID, CreateKnowledgeRequest{
			ChannelID: channelID,
			Title:     entry.Title,
			Content:   entry.Content,
			Tags:      []string{"decision"},
			Source:    "auto",
		})
		if err != nil {
			slog.Warn("failed to import decision entry", "title", entry.Title, "error", err)
			continue
		}
		count++
	}

	slog.Info("imported decisions to knowledge", "channel_id", channelID, "count", count)
	return count, nil
}

// decisionBlock represents a parsed decision from decisions.md.
type decisionBlock struct {
	Title     string
	Content   string
	SourceRef string
}

// parseDecisionBlocks parses the legacy decisions.md format:
//
//	## 2026-06-13: Title here
//
//	Content lines...
//	---
func parseDecisionBlocks(raw string) []decisionBlock {
	var blocks []decisionBlock
	scanner := bufio.NewScanner(strings.NewReader(raw))

	var current *decisionBlock
	var contentLines []string
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			// Save previous block if any.
			if current != nil {
				current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
				blocks = append(blocks, *current)
			}
			current = &decisionBlock{
				Title:     strings.TrimPrefix(line, "## "),
				SourceRef: fmt.Sprintf("decisions.md#L%d", lineNum),
			}
			contentLines = nil
		} else if strings.TrimSpace(line) == "---" {
			// End of block.
			if current != nil {
				current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
				blocks = append(blocks, *current)
				current = nil
				contentLines = nil
			}
		} else if current != nil {
			contentLines = append(contentLines, line)
		}
	}

	// Save trailing block without --- delimiter.
	if current != nil {
		current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
		blocks = append(blocks, *current)
	}

	return blocks
}

// SearchByEmbedding searches using a pre-computed embedding vector directly.
// Used by the daemon to inject relevant knowledge into the system prompt.
func (s *KnowledgeService) SearchByEmbedding(ctx context.Context, embedding []float32, channelID string, limit int) ([]KnowledgeEntry, error) {
	vecStr := vectorToString(embedding)
	rows, err := s.pool.Query(ctx,
		`SELECT k.title, k.content, 1 - (k.embedding <=> $1::vector) AS similarity
		 FROM knowledge k
		 WHERE k.channel_id = $2 AND k.embedding IS NOT NULL
		 ORDER BY k.embedding <=> $1::vector
		 LIMIT $3`,
		vecStr, channelID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("embedding search: %w", err)
	}
	defer rows.Close()

	var entries []KnowledgeEntry
	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(&e.Title, &e.Content, &e.Similarity); err != nil {
			return nil, fmt.Errorf("scan embedding result: %w", err)
		}
		e.ChannelID = channelID
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
