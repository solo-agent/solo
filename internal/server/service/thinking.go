package service

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxThinkingDepth    = 6
	maxThinkingChildren = 8
	maxAutoSplits       = 3
)

var (
	ErrThinkingNotFound  = errors.New("thinking space or node not found")
	ErrThinkingLimit     = errors.New("thinking node limit reached")
	ErrThinkingReturning = errors.New("thinking node return is already in progress")
	ErrThinkingReturned  = errors.New("returned thinking nodes are closed")
	ErrThinkingBusy      = errors.New("thinking node has an active Agent run")
	ErrThinkingChildren  = errors.New("all child nodes must be returned first")
	ErrThinkingPreparing = errors.New("thinking node is waiting for its fork handoff")
	ErrThinkingDuplicate = errors.New("a sibling node with this title already exists")
	splitDirective       = regexp.MustCompile(`(?m)^\s*\[\[split:\s*([^\]\n]{1,100})\]\]\s*$`)
)

type ThinkingNode struct {
	ID                  string     `json:"id"`
	SpaceID             string     `json:"space_id"`
	ParentID            string     `json:"parent_id,omitempty"`
	AgentID             string     `json:"agent_id,omitempty"`
	AgentName           string     `json:"agent_name,omitempty"`
	AgentSessionID      string     `json:"agent_session_id,omitempty"`
	Title               string     `json:"title"`
	Source              string     `json:"source"`
	CheckpointHandoff   string     `json:"checkpoint_handoff,omitempty"`
	CheckpointHandoffAt *time.Time `json:"checkpoint_handoff_at,omitempty"`
	InheritedHandoff    string     `json:"inherited_handoff,omitempty"`
	ForkHandoffPending  bool       `json:"fork_handoff_pending"`
	ForkHandoffAt       *time.Time `json:"fork_handoff_at,omitempty"`
	ReturnedHandoff     string     `json:"returned_handoff,omitempty"`
	ReturningAt         *time.Time `json:"returning_at,omitempty"`
	ReturnedAt          *time.Time `json:"returned_at,omitempty"`
	Depth               int        `json:"depth"`
	SortOrder           int        `json:"sort_order"`
	MessageCount        int        `json:"message_count"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type ThinkingSpace struct {
	ID        string         `json:"id"`
	ChannelID string         `json:"channel_id"`
	Nodes     []ThinkingNode `json:"nodes"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ThinkingReturnMessage struct {
	ID        string
	Content   string
	CreatedAt time.Time
}

type ThinkingService struct {
	pool *pgxpool.Pool
}

func NewThinkingService(pool *pgxpool.Pool) *ThinkingService {
	return &ThinkingService{pool: pool}
}

func (s *ThinkingService) Get(ctx context.Context, channelID string) (*ThinkingSpace, error) {
	var space ThinkingSpace
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, channel_id::text, created_at, updated_at
		  FROM thinking_spaces WHERE channel_id = $1`, channelID,
	).Scan(&space.ID, &space.ChannelID, &space.CreatedAt, &space.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrThinkingNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT n.id::text, n.space_id::text, COALESCE(n.parent_id::text, ''),
		       COALESCE(n.agent_id::text, ''), COALESCE(a.name, ''), COALESCE(n.agent_session_id::text, ''), n.title, n.source,
		       n.checkpoint_handoff, n.checkpoint_handoff_at, n.inherited_handoff,
		       n.fork_handoff_pending, n.fork_handoff_at, n.returned_handoff, n.returning_at, n.returned_at,
		       n.depth, n.sort_order,
		       (SELECT COUNT(*) FROM messages m WHERE m.thinking_node_id = n.id
		          AND COALESCE(m.is_deleted, false) = false),
		       n.created_at, n.updated_at
		  FROM thinking_nodes n
		  LEFT JOIN agents a ON a.id = n.agent_id
		 WHERE n.space_id = $1
		 ORDER BY n.depth, n.sort_order, n.created_at`, space.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	space.Nodes = []ThinkingNode{}
	for rows.Next() {
		var node ThinkingNode
		if err := rows.Scan(
			&node.ID, &node.SpaceID, &node.ParentID, &node.AgentID, &node.AgentName, &node.AgentSessionID,
			&node.Title, &node.Source, &node.CheckpointHandoff, &node.CheckpointHandoffAt, &node.InheritedHandoff,
			&node.ForkHandoffPending, &node.ForkHandoffAt, &node.ReturnedHandoff,
			&node.ReturningAt, &node.ReturnedAt, &node.Depth, &node.SortOrder,
			&node.MessageCount, &node.CreatedAt, &node.UpdatedAt,
		); err != nil {
			return nil, err
		}
		space.Nodes = append(space.Nodes, node)
	}
	return &space, rows.Err()
}

type thinkingTeamAgent struct {
	id   string
	name string
}

type thinkingTeamEdge struct {
	from string
	to   string
}

func (s *ThinkingService) Ensure(ctx context.Context, channelID, actorID string) (*ThinkingSpace, error) {
	if existing, err := s.Get(ctx, channelID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrThinkingNotFound) {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var channelName string
	if err := tx.QueryRow(ctx, `SELECT name FROM channels WHERE id = $1 AND is_archived = false`, channelID).Scan(&channelName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThinkingNotFound
		}
		return nil, err
	}

	spaceID := uuid.NewString()
	var createdID string
	err = tx.QueryRow(ctx, `
		INSERT INTO thinking_spaces (id, channel_id, created_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (channel_id) DO NOTHING
		RETURNING id::text`, spaceID, channelID, actorID).Scan(&createdID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return s.Get(ctx, channelID)
	}
	if err != nil {
		return nil, err
	}

	agents, edges, err := loadThinkingTeam(ctx, tx, channelID)
	if err != nil {
		return nil, err
	}
	rootAgent, children := chooseThinkingTeam(agents, edges)
	rootID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO thinking_nodes (id, space_id, agent_id, title, source, created_by)
		VALUES ($1, $2, $3, $4, 'root', $5)`,
		rootID, createdID, nullableUUID(rootAgent.id), channelName, actorID,
	); err != nil {
		return nil, err
	}
	for i, child := range children {
		if _, err := tx.Exec(ctx, `
			INSERT INTO thinking_nodes
			    (id, space_id, parent_id, agent_id, title, source, depth, sort_order, created_by)
			VALUES ($1, $2, $3, $4, $5, 'team', 1, $6, $7)`,
			uuid.NewString(), createdID, rootID, child.id, child.name, i, actorID,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, channelID)
}

func loadThinkingTeam(ctx context.Context, tx pgx.Tx, channelID string) ([]thinkingTeamAgent, []thinkingTeamEdge, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id::text, a.name
		  FROM channel_members cm
		  JOIN agents a ON a.id = cm.member_id AND a.is_active = true
		 WHERE cm.channel_id = $1 AND cm.member_type = 'agent'
		 ORDER BY cm.joined_at, a.name`, channelID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	agents := []thinkingTeamAgent{}
	for rows.Next() {
		var agent thinkingTeamAgent
		if err := rows.Scan(&agent.id, &agent.name); err != nil {
			return nil, nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	edgeRows, err := tx.Query(ctx, `
		SELECT r.from_agent_id::text, r.to_agent_id::text
		  FROM agent_relationships r
		  JOIN channel_members f ON f.channel_id = $1 AND f.member_type = 'agent' AND f.member_id = r.from_agent_id
		  JOIN channel_members t ON t.channel_id = $1 AND t.member_type = 'agent' AND t.member_id = r.to_agent_id
		 WHERE r.rel_type = 'assigns_to'`, channelID)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()
	edges := []thinkingTeamEdge{}
	for edgeRows.Next() {
		var edge thinkingTeamEdge
		if err := edgeRows.Scan(&edge.from, &edge.to); err != nil {
			return nil, nil, err
		}
		edges = append(edges, edge)
	}
	return agents, edges, edgeRows.Err()
}

func chooseThinkingTeam(agents []thinkingTeamAgent, edges []thinkingTeamEdge) (thinkingTeamAgent, []thinkingTeamAgent) {
	if len(agents) == 0 {
		return thinkingTeamAgent{}, nil
	}
	byID := make(map[string]thinkingTeamAgent, len(agents))
	incoming := make(map[string]int, len(agents))
	outgoing := make(map[string]int, len(agents))
	for _, agent := range agents {
		byID[agent.id] = agent
	}
	for _, edge := range edges {
		if _, ok := byID[edge.from]; !ok {
			continue
		}
		if _, ok := byID[edge.to]; !ok {
			continue
		}
		incoming[edge.to]++
		outgoing[edge.from]++
	}
	candidates := append([]thinkingTeamAgent(nil), agents...)
	sort.SliceStable(candidates, func(i, j int) bool {
		leftRoot, rightRoot := incoming[candidates[i].id] == 0, incoming[candidates[j].id] == 0
		if leftRoot != rightRoot {
			return leftRoot
		}
		return outgoing[candidates[i].id] > outgoing[candidates[j].id]
	})
	root := candidates[0]
	children := []thinkingTeamAgent{}
	for _, edge := range edges {
		if edge.from == root.id {
			if child, ok := byID[edge.to]; ok {
				children = append(children, child)
			}
		}
	}
	if len(children) == 0 && len(agents) > 1 {
		for _, agent := range agents {
			if agent.id != root.id {
				children = append(children, agent)
			}
		}
	}
	return root, children
}

func (s *ThinkingService) GetNodeForChannel(ctx context.Context, channelID, nodeID string) (*ThinkingNode, error) {
	var node ThinkingNode
	err := s.pool.QueryRow(ctx, `
		SELECT n.id::text, n.space_id::text, COALESCE(n.parent_id::text, ''),
		       COALESCE(n.agent_id::text, ''), COALESCE(a.name, ''), COALESCE(n.agent_session_id::text, ''), n.title, n.source,
		       n.checkpoint_handoff, n.checkpoint_handoff_at, n.inherited_handoff,
		       n.fork_handoff_pending, n.fork_handoff_at, n.returned_handoff, n.returning_at, n.returned_at,
		       n.depth, n.sort_order,
		       (SELECT COUNT(*) FROM messages m WHERE m.thinking_node_id = n.id
		          AND COALESCE(m.is_deleted, false) = false),
		       n.created_at, n.updated_at
		  FROM thinking_nodes n
		  JOIN thinking_spaces s ON s.id = n.space_id AND s.channel_id = $1
		  LEFT JOIN agents a ON a.id = n.agent_id
		 WHERE n.id = $2`, channelID, nodeID,
	).Scan(
		&node.ID, &node.SpaceID, &node.ParentID, &node.AgentID, &node.AgentName, &node.AgentSessionID,
		&node.Title, &node.Source, &node.CheckpointHandoff, &node.CheckpointHandoffAt, &node.InheritedHandoff,
		&node.ForkHandoffPending, &node.ForkHandoffAt, &node.ReturnedHandoff,
		&node.ReturningAt, &node.ReturnedAt, &node.Depth, &node.SortOrder,
		&node.MessageCount, &node.CreatedAt, &node.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrThinkingNotFound
	}
	return &node, err
}

func (s *ThinkingService) CreateChild(ctx context.Context, channelID, parentID, title, actorID, source string) (*ThinkingNode, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 100 {
		return nil, errors.New("thinking node title must be between 1 and 100 characters")
	}
	if source != "auto" {
		source = "manual"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var parent ThinkingNode
	err = tx.QueryRow(ctx, `
		SELECT n.id::text, n.space_id::text, COALESCE(n.agent_id::text, ''),
		       n.fork_handoff_pending, n.returning_at, n.returned_at, n.depth
		  FROM thinking_nodes n
		  JOIN thinking_spaces s ON s.id = n.space_id AND s.channel_id = $1
		 WHERE n.id = $2
		 FOR UPDATE OF n`, channelID, parentID,
	).Scan(&parent.ID, &parent.SpaceID, &parent.AgentID, &parent.ForkHandoffPending, &parent.ReturningAt, &parent.ReturnedAt, &parent.Depth)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrThinkingNotFound
	}
	if err != nil {
		return nil, err
	}
	if parent.ReturnedAt != nil {
		return nil, ErrThinkingReturned
	}
	if parent.ReturningAt != nil {
		return nil, ErrThinkingReturning
	}
	if parent.ForkHandoffPending {
		return nil, ErrThinkingPreparing
	}
	if parent.Depth >= maxThinkingDepth {
		return nil, ErrThinkingLimit
	}
	if parent.Depth == 0 {
		var hasTeamChildren bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM thinking_nodes
				 WHERE parent_id = $1 AND source = 'team'
			)`, parentID).Scan(&hasTeamChildren); err != nil {
			return nil, err
		}
		if hasTeamChildren {
			return nil, ErrThinkingLimit
		}
	}
	var childCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM thinking_nodes WHERE parent_id = $1`, parentID).Scan(&childCount); err != nil {
		return nil, err
	}
	if childCount >= maxThinkingChildren {
		return nil, ErrThinkingLimit
	}
	nodeID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO thinking_nodes
		    (id, space_id, parent_id, agent_id, title, source, fork_handoff_pending, depth, sort_order, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8, $9)`,
		nodeID, parent.SpaceID, parent.ID, nullableUUID(parent.AgentID), title, source,
		parent.Depth+1, childCount, actorID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_thinking_nodes_sibling_title" {
			return nil, ErrThinkingDuplicate
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetNodeForChannel(ctx, channelID, nodeID)
}

func (s *ThinkingService) SaveCheckpointHandoff(ctx context.Context, channelID, nodeID, agentID, handoff string) (*ThinkingNode, error) {
	handoff = strings.TrimSpace(handoff)
	if handoff == "" {
		return nil, errors.New("checkpoint handoff is empty")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE thinking_nodes node
		   SET checkpoint_handoff = $4, checkpoint_handoff_at = now(), updated_at = now()
		  FROM thinking_spaces space
		 WHERE node.id = $2 AND node.space_id = space.id AND space.channel_id = $1
		   AND node.agent_id = $3 AND node.returning_at IS NULL AND node.returned_at IS NULL
		   AND node.fork_handoff_pending = false`, channelID, nodeID, agentID, handoff)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrThinkingNotFound
	}
	return s.GetNodeForChannel(ctx, channelID, nodeID)
}

func (s *ThinkingService) CompleteForkHandoff(ctx context.Context, channelID, parentID, childID, agentID, handoff string) (*ThinkingNode, error) {
	handoff = strings.TrimSpace(handoff)
	if handoff == "" {
		return nil, errors.New("fork handoff is empty")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE thinking_nodes child
		   SET inherited_handoff = $5, fork_handoff_pending = false,
		       fork_handoff_at = now(), updated_at = now()
		  FROM thinking_nodes parent, thinking_spaces space
		 WHERE child.id = $3 AND child.parent_id = parent.id AND parent.id = $2
		   AND child.space_id = space.id AND space.channel_id = $1
		   AND parent.agent_id = $4 AND parent.returning_at IS NULL AND parent.returned_at IS NULL
		   AND child.fork_handoff_pending = true AND child.returned_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM messages message WHERE message.thinking_node_id = child.id)`,
		channelID, parentID, childID, agentID, handoff)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrThinkingNotFound
	}
	return s.GetNodeForChannel(ctx, channelID, childID)
}

func (s *ThinkingService) BeginReturn(ctx context.Context, channelID, nodeID string) (*ThinkingNode, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var node ThinkingNode
	var messageCount int
	err = tx.QueryRow(ctx, `
		SELECT n.id::text, COALESCE(n.parent_id::text, ''), COALESCE(n.agent_id::text, ''),
		       n.fork_handoff_pending, n.returning_at, n.returned_at,
		       (SELECT COUNT(*) FROM messages m WHERE m.thinking_node_id = n.id AND COALESCE(m.is_deleted, false) = false)
		  FROM thinking_nodes n
		  JOIN thinking_spaces s ON s.id = n.space_id AND s.channel_id = $1
		 WHERE n.id = $2
		 FOR UPDATE OF n`, channelID, nodeID,
	).Scan(&node.ID, &node.ParentID, &node.AgentID, &node.ForkHandoffPending, &node.ReturningAt, &node.ReturnedAt, &messageCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrThinkingNotFound
	}
	if err != nil {
		return nil, err
	}
	if node.ReturnedAt != nil {
		return nil, ErrThinkingReturned
	}
	if node.ReturningAt != nil {
		return nil, ErrThinkingReturning
	}
	if node.ForkHandoffPending {
		return nil, ErrThinkingPreparing
	}
	if node.ParentID == "" || node.AgentID == "" || messageCount == 0 {
		return nil, errors.New("node needs a parent, assigned Agent, and conversation before return")
	}
	var hasActiveChildren bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM thinking_nodes child
			 WHERE child.parent_id = $1 AND child.returned_at IS NULL
		)`, nodeID).Scan(&hasActiveChildren); err != nil {
		return nil, err
	}
	if hasActiveChildren {
		return nil, ErrThinkingChildren
	}
	var hasActiveRun bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM agent_runs
			 WHERE thinking_node_id = $1
			   AND status IN ('queued', 'thinking', 'running', 'streaming', 'waiting_input', 'waiting_approval')
		)`, nodeID).Scan(&hasActiveRun); err != nil {
		return nil, err
	}
	if hasActiveRun {
		return nil, ErrThinkingBusy
	}
	if _, err := tx.Exec(ctx, `
		UPDATE thinking_nodes SET returning_at = now(), updated_at = now()
		 WHERE id = $1`, nodeID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetNodeForChannel(ctx, channelID, nodeID)
}

func (s *ThinkingService) CompleteReturn(ctx context.Context, channelID, nodeID, agentID, handoff string) (*ThinkingNode, *ThinkingReturnMessage, error) {
	handoff = strings.TrimSpace(handoff)
	if handoff == "" {
		return nil, nil, errors.New("handoff is empty")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	var node ThinkingNode
	err = tx.QueryRow(ctx, `
		SELECT n.id::text, COALESCE(n.parent_id::text, ''), COALESCE(n.agent_id::text, ''),
		       n.title, n.returning_at, n.returned_at
		  FROM thinking_nodes n
		  JOIN thinking_spaces s ON s.id = n.space_id AND s.channel_id = $1
		 WHERE n.id = $2
		 FOR UPDATE OF n`, channelID, nodeID,
	).Scan(&node.ID, &node.ParentID, &node.AgentID, &node.Title, &node.ReturningAt, &node.ReturnedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrThinkingNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if node.ReturnedAt != nil {
		return nil, nil, ErrThinkingReturned
	}
	if node.ReturningAt == nil || node.AgentID != agentID {
		return nil, nil, ErrThinkingReturning
	}
	if _, err := tx.Exec(ctx, `
		UPDATE thinking_nodes
		   SET returned_handoff = $2, returning_at = NULL,
		       returned_at = now(), updated_at = now()
		 WHERE id = $1`, nodeID, handoff); err != nil {
		return nil, nil, err
	}
	returnMessage := &ThinkingReturnMessage{
		ID:        uuid.NewString(),
		Content:   "Handoff returned from " + node.Title + ":\n" + handoff,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages
		    (id, channel_id, thinking_node_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		VALUES ($1, $2, $3, 'system', '00000000-0000-0000-0000-000000000000', $4, 'thinking_handoff', now(), now())`,
		returnMessage.ID, channelID, node.ParentID, returnMessage.Content); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	updated, err := s.GetNodeForChannel(ctx, channelID, nodeID)
	return updated, returnMessage, err
}

func (s *ThinkingService) CancelReturn(ctx context.Context, nodeID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE thinking_nodes SET returning_at = NULL, updated_at = now()
		 WHERE id = $1 AND returning_at IS NOT NULL AND returned_at IS NULL`, nodeID)
	return err == nil && tag.RowsAffected() == 1, err
}

func ExtractThinkingSplits(content string) (string, []string) {
	titles := []string{}
	clean := splitDirective.ReplaceAllStringFunc(content, func(marker string) string {
		if len(titles) >= maxAutoSplits {
			return marker
		}
		match := splitDirective.FindStringSubmatch(marker)
		title := strings.TrimSpace(match[1])
		if title != "" {
			titles = append(titles, title)
		}
		return ""
	})
	return strings.TrimSpace(clean), titles
}

type ThinkingHandoffProtocol struct {
	Kind     string
	TargetID string
	Content  string
}

func ParseThinkingHandoffProtocol(content string) (ThinkingHandoffProtocol, bool) {
	first, body, found := strings.Cut(strings.TrimSpace(content), "\n")
	if !found || strings.TrimSpace(body) == "" {
		return ThinkingHandoffProtocol{}, false
	}
	protocol := ThinkingHandoffProtocol{Content: strings.TrimSpace(body)}
	switch first {
	case "[[handoff:checkpoint]]":
		protocol.Kind = "checkpoint"
	case "[[handoff:return]]":
		protocol.Kind = "return"
	default:
		const prefix = "[[handoff:fork:"
		if !strings.HasPrefix(first, prefix) || !strings.HasSuffix(first, "]]") {
			return ThinkingHandoffProtocol{}, false
		}
		protocol.Kind = "fork"
		protocol.TargetID = strings.TrimSuffix(strings.TrimPrefix(first, prefix), "]]")
		if _, err := uuid.Parse(protocol.TargetID); err != nil {
			return ThinkingHandoffProtocol{}, false
		}
	}
	return protocol, true
}
