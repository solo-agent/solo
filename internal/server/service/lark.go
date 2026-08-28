package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"

	"github.com/solo-ai/solo/internal/auth"
	"github.com/solo-ai/solo/internal/realtime"
)

const larkMessageLimit = 10_000

type LarkService struct {
	pool     *pgxpool.Pool
	hub      realtime.Broadcaster
	agentSvc *AgentService
	client   *http.Client
	qr       *larkQRState
}

type LarkBinding struct {
	ID               string `json:"id"`
	WorkspaceID      string `json:"workspace_id"`
	ChannelID        string `json:"channel_id"`
	AgentID          string `json:"agent_id"`
	Platform         string `json:"platform"`
	AppID            string `json:"app_id"`
	ExternalChatID   string `json:"external_chat_id,omitempty"`
	ExternalChatType string `json:"external_chat_type,omitempty"`
	LastStatus       string `json:"last_status,omitempty"`
	LastError        string `json:"last_error,omitempty"`
	ConnectionMode   string `json:"connection_mode"`
	ConnectionStatus string `json:"connection_status"`
	ConnectionError  string `json:"connection_error,omitempty"`
	UpdatedAt        string `json:"updated_at"`
}

type SaveLarkBindingInput struct {
	WorkspaceID       string
	ChannelID         string
	AgentID           string
	Platform          string
	AppID             string
	AppSecret         string
	VerificationToken string
	UserID            string
}

type LarkInboundEvent struct {
	EventID      string
	MessageID    string
	ChatID       string
	ChatType     string
	SenderOpenID string
	SenderName   string
	Text         string
	Mentioned    bool
}

func NewLarkService(ctx context.Context, pool *pgxpool.Pool, hub realtime.Broadcaster, agentSvc *AgentService) *LarkService {
	s := &LarkService{
		pool: pool, hub: hub, agentSvc: agentSvc, client: &http.Client{Timeout: 10 * time.Second},
		qr: newLarkQRState(ctx),
	}
	go s.restoreLarkWebsocketBindings(ctx)
	return s
}

func (s *LarkService) GetBinding(ctx context.Context, workspaceID string) (*LarkBinding, error) {
	var b LarkBinding
	var updated time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT b.id, b.workspace_id, b.channel_id, b.agent_id, b.platform, b.app_id,
		       COALESCE(b.external_chat_id, ''), COALESCE(b.external_chat_type, ''),
		       COALESCE(d.status, ''), COALESCE(d.last_error, ''),
		       b.connection_mode, b.connection_status, COALESCE(b.connection_error, ''), b.updated_at
		  FROM lark_bindings b
		  LEFT JOIN LATERAL (
		       SELECT status, last_error FROM lark_deliveries
		        WHERE binding_id = b.id AND direction = 'outbound'
		        ORDER BY created_at DESC LIMIT 1
		  ) d ON true
		 WHERE b.workspace_id = $1`, workspaceID).Scan(
		&b.ID, &b.WorkspaceID, &b.ChannelID, &b.AgentID, &b.Platform, &b.AppID,
		&b.ExternalChatID, &b.ExternalChatType, &b.LastStatus, &b.LastError,
		&b.ConnectionMode, &b.ConnectionStatus, &b.ConnectionError, &updated,
	)
	if err != nil {
		return nil, err
	}
	b.UpdatedAt = updated.UTC().Format(time.RFC3339)
	return &b, nil
}

func (s *LarkService) SaveBinding(ctx context.Context, in SaveLarkBindingInput) (*LarkBinding, error) {
	if in.Platform != "feishu" && in.Platform != "lark" {
		return nil, errors.New("platform must be feishu or lark")
	}
	if strings.TrimSpace(in.AppID) == "" {
		return nil, errors.New("app ID is required")
	}
	var valid bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM channels c
		  JOIN agents a ON a.id = $3
		  JOIN channel_members cm ON cm.channel_id = c.id AND cm.member_type = 'agent' AND cm.member_id = a.id
		  WHERE c.id = $2 AND c.workspace_id = $1 AND a.is_active = true
		)`, in.WorkspaceID, in.ChannelID, in.AgentID).Scan(&valid); err != nil || !valid {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("channel and Agent must belong to this Workspace")
	}

	var existingSecret, existingTokenHash string
	err := s.pool.QueryRow(ctx, `SELECT app_secret_encrypted, verification_token_hash FROM lark_bindings WHERE workspace_id = $1`, in.WorkspaceID).Scan(&existingSecret, &existingTokenHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if strings.TrimSpace(in.AppSecret) == "" {
		if existingSecret == "" {
			return nil, errors.New("app secret is required")
		}
	} else {
		existingSecret, err = encryptLarkSecret(in.AppSecret)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(in.VerificationToken) == "" {
		if existingTokenHash == "" {
			return nil, errors.New("verification token is required")
		}
	} else {
		existingTokenHash = hashLarkToken(in.VerificationToken)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO lark_bindings (workspace_id, channel_id, agent_id, platform, app_id, app_secret_encrypted, verification_token_hash, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (workspace_id) DO UPDATE SET
		  channel_id = EXCLUDED.channel_id, agent_id = EXCLUDED.agent_id,
		  platform = EXCLUDED.platform, app_id = EXCLUDED.app_id,
		  app_secret_encrypted = EXCLUDED.app_secret_encrypted,
		  verification_token_hash = EXCLUDED.verification_token_hash,
		  connection_mode = 'callback', connection_status = 'connected', connection_error = NULL,
		  external_chat_id = CASE
		    WHEN lark_bindings.channel_id = EXCLUDED.channel_id
		     AND lark_bindings.platform = EXCLUDED.platform
		     AND lark_bindings.app_id = EXCLUDED.app_id
		    THEN lark_bindings.external_chat_id ELSE NULL END,
		  external_chat_type = CASE
		    WHEN lark_bindings.channel_id = EXCLUDED.channel_id
		     AND lark_bindings.platform = EXCLUDED.platform
		     AND lark_bindings.app_id = EXCLUDED.app_id
		    THEN lark_bindings.external_chat_type ELSE NULL END,
		  updated_at = now()`,
		in.WorkspaceID, in.ChannelID, in.AgentID, in.Platform, strings.TrimSpace(in.AppID), existingSecret, existingTokenHash, in.UserID)
	if err != nil {
		return nil, err
	}
	return s.GetBinding(ctx, in.WorkspaceID)
}

func (s *LarkService) DeleteBinding(ctx context.Context, workspaceID string) error {
	var bindingID string
	_ = s.pool.QueryRow(ctx, `SELECT id FROM lark_bindings WHERE workspace_id = $1`, workspaceID).Scan(&bindingID)
	_, err := s.pool.Exec(ctx, `DELETE FROM lark_bindings WHERE workspace_id = $1`, workspaceID)
	if err == nil && bindingID != "" {
		s.stopLarkWebsocket(bindingID)
	}
	return err
}

func (s *LarkService) IsWorkspaceAdmin(ctx context.Context, workspaceID, userID string) bool {
	var role string
	if err := s.pool.QueryRow(ctx, `SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, userID).Scan(&role); err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

func (s *LarkService) VerifyCallback(ctx context.Context, bindingID, signature, token string) (*LarkBinding, error) {
	if !hmac.Equal([]byte(signature), []byte(larkCallbackSignature(bindingID))) {
		return nil, errors.New("invalid callback signature")
	}
	var workspaceID, tokenHash string
	if err := s.pool.QueryRow(ctx, `SELECT workspace_id, verification_token_hash FROM lark_bindings WHERE id = $1`, bindingID).Scan(&workspaceID, &tokenHash); err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(tokenHash), []byte(hashLarkToken(token))) {
		return nil, errors.New("invalid verification token")
	}
	return s.GetBinding(ctx, workspaceID)
}

func (s *LarkService) CallbackSignature(bindingID string) string {
	return larkCallbackSignature(bindingID)
}

func (s *LarkService) HandleInbound(ctx context.Context, b *LarkBinding, event LarkInboundEvent) error {
	text := strings.TrimSpace(event.Text)
	if text == "" || len([]rune(text)) > larkMessageLimit {
		return errors.New("unsupported or oversized message")
	}
	if event.ChatType != "p2p" && !event.Mentioned {
		return nil
	}
	if strings.TrimSpace(event.ChatID) == "" || strings.TrimSpace(event.SenderOpenID) == "" {
		return errors.New("missing chat or sender identity")
	}
	senderName := strings.TrimSpace(event.SenderName)
	senderAvatar := ""
	if senderName == "" {
		senderName, senderAvatar = s.larkSenderProfile(ctx, b, event.SenderOpenID)
	}
	if senderName == "" {
		suffix := event.SenderOpenID
		if len(suffix) > 4 {
			suffix = suffix[len(suffix)-4:]
		}
		senderName = "飞书成员 " + suffix
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	deliveryID := uuid.New().String()
	externalID := event.MessageID
	if externalID == "" {
		externalID = event.EventID
	}
	if externalID == "" {
		return errors.New("missing event identity")
	}
	var inserted string
	err = tx.QueryRow(ctx, `
		INSERT INTO lark_deliveries (id, binding_id, direction, external_message_id)
		VALUES ($1,$2,'inbound',$3)
		ON CONFLICT DO NOTHING RETURNING id`, deliveryID, b.ID, externalID).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE lark_bindings SET external_chat_id=$2, external_chat_type=$3, updated_at=now() WHERE id=$1 AND external_chat_id IS NULL`, b.ID, event.ChatID, event.ChatType); err != nil {
		return err
	}

	senderID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("solo:lark:"+b.Platform+":"+event.SenderOpenID)).String()
	metadata := map[string]any{
		"source": "lark", "platform": b.Platform, "lark_binding_id": b.ID,
		"external_chat_id": event.ChatID, "external_message_id": externalID,
		"external_sender_name": senderName, "external_sender_avatar": senderAvatar, "trust": "external",
	}
	metadataJSON, _ := json.Marshal(metadata)
	messageID := uuid.New().String()
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages (id, channel_id, sender_type, sender_id, content, mentioned_agent_ids, metadata, created_at, updated_at)
		VALUES ($1,$2,'external',$3,$4,$5::uuid[],$6::jsonb,now(),now())`,
		messageID, b.ChannelID, senderID, text, "{"+b.AgentID+"}", metadataJSON); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE lark_deliveries SET solo_message_id=$2, status='sent', attempts=1, updated_at=now() WHERE id=$1`, deliveryID, messageID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if s.hub != nil {
		s.hub.BroadcastToChannel(b.ChannelID, realtime.Envelope("message.new", map[string]any{
			"id": messageID, "channel_id": b.ChannelID, "sender_type": "external", "sender_id": senderID,
			"sender_name": senderName, "sender_avatar": senderAvatar, "content": text, "content_type": "text",
			"metadata": metadata, "mentioned_agent_ids": []string{b.AgentID}, "created_at": time.Now().UTC().Format(time.RFC3339),
		}))
	}
	if s.agentSvc != nil {
		go s.agentSvc.TriggerAgentResponse(context.Background(), b.ChannelID, messageID, "external", senderID, []string{b.AgentID}, true, nil)
	}
	return nil
}

func (s *LarkService) DeliverAgentReply(ctx context.Context, messageID string) {
	var bindingID, platform, appID, encryptedSecret, chatID, content, agentName string
	err := s.pool.QueryRow(ctx, `
		SELECT b.id, b.platform, b.app_id, b.app_secret_encrypted,
		       COALESCE(NULLIF(trigger.metadata->>'external_chat_id',''), b.external_chat_id, ''),
		       m.content, COALESCE(a.name,'Agent')
		  FROM messages m
		  JOIN agents a ON a.id = m.sender_id
		  JOIN agent_runs r ON r.id = NULLIF(m.metadata->>'agent_run_id','')::uuid
		  JOIN messages trigger ON trigger.id = r.trigger_message_id
		  JOIN lark_bindings b ON b.id = NULLIF(trigger.metadata->>'lark_binding_id','')::uuid
		 WHERE m.id=$1 AND m.sender_type='agent' AND m.channel_id=b.channel_id`, messageID).Scan(
		&bindingID, &platform, &appID, &encryptedSecret, &chatID, &content, &agentName)
	if err != nil || chatID == "" {
		return
	}

	var deliveryID string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO lark_deliveries (binding_id, direction, solo_message_id)
		VALUES ($1,'outbound',$2) ON CONFLICT DO NOTHING RETURNING id`, bindingID, messageID).Scan(&deliveryID)
	if err != nil {
		return
	}
	externalID, sendErr := s.sendReply(ctx, platform, appID, encryptedSecret, chatID, agentName, content)
	status, lastError := "sent", ""
	if sendErr != nil {
		status, lastError = "failed", sendErr.Error()
	}
	_, _ = s.pool.Exec(ctx, `UPDATE lark_deliveries SET external_message_id=$2, status=$3, attempts=attempts+1, last_error=NULLIF($4,''), updated_at=now() WHERE id=$1`, deliveryID, externalID, status, lastError)
	if sendErr != nil {
		slog.Warn("failed to deliver Agent reply to Lark", "message_id", messageID, "error", sendErr)
	}
}

func (s *LarkService) larkSenderProfile(ctx context.Context, b *LarkBinding, openID string) (string, string) {
	// ponytail: fetch per message; cache by binding/openID only when external traffic makes this measurable.
	var encryptedSecret string
	if err := s.pool.QueryRow(ctx, `SELECT app_secret_encrypted FROM lark_bindings WHERE id=$1`, b.ID).Scan(&encryptedSecret); err != nil {
		return "", ""
	}
	secret, err := decryptLarkSecret(encryptedSecret)
	if err != nil {
		return "", ""
	}
	opts := []lark.ClientOptionFunc{lark.WithReqTimeout(5 * time.Second)}
	if b.Platform == "lark" {
		opts = append(opts, lark.WithOpenBaseUrl(lark.LarkBaseUrl))
	}
	client := lark.NewClient(b.AppID, secret, opts...)
	resp, err := client.Contact.User.Get(ctx, larkcontact.NewGetUserReqBuilder().UserId(openID).UserIdType("open_id").Build())
	if err != nil || !resp.Success() || resp.Data == nil || resp.Data.User == nil {
		return "", ""
	}
	user := resp.Data.User
	avatar := ""
	if user.Avatar != nil {
		avatar = value(user.Avatar.Avatar72)
	}
	return value(user.Name), avatar
}

func (s *LarkService) RetryFailed(ctx context.Context, bindingID string) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT solo_message_id FROM lark_deliveries WHERE binding_id=$1 AND direction='outbound' AND status='failed' AND solo_message_id IS NOT NULL ORDER BY created_at`, bindingID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		_, _ = s.pool.Exec(ctx, `DELETE FROM lark_deliveries WHERE binding_id=$1 AND direction='outbound' AND solo_message_id=$2 AND status='failed'`, bindingID, id)
		s.DeliverAgentReply(ctx, id)
	}
	return len(ids), rows.Err()
}

func (s *LarkService) sendReply(ctx context.Context, platform, appID, encryptedSecret, chatID, agentName, content string) (string, error) {
	secret, err := decryptLarkSecret(encryptedSecret)
	if err != nil {
		return "", err
	}
	base := "https://open.feishu.cn"
	if platform == "lark" {
		base = "https://open.larksuite.com"
	}
	tokenBody, _ := json.Marshal(map[string]string{"app_id": appID, "app_secret": secret})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(tokenBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var tokenResp struct {
		Code  int    `json:"code"`
		Msg   string `json:"msg"`
		Token string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tokenResp); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || tokenResp.Code != 0 || tokenResp.Token == "" {
		return "", fmt.Errorf("tenant token rejected: %s", tokenResp.Msg)
	}
	messageType, messageContent := larkReplyPayload(agentName, content)
	body, _ := json.Marshal(map[string]string{"receive_id": chatID, "msg_type": messageType, "content": messageContent})
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, base+"/open-apis/im/v1/messages?receive_id_type=chat_id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var sendResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&sendResp); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || sendResp.Code != 0 {
		return "", fmt.Errorf("message rejected: %s", sendResp.Msg)
	}
	return sendResp.Data.MessageID, nil
}

var larkMarkdownPattern = regexp.MustCompile("(?m)(^#{1,6}[ \\t]|^[ \\t]*[-*+][ \\t]|^[ \\t]*\\d+\\.[ \\t]|^>[ \\t]|\\*\\*[^*\\n]+\\*\\*|__[^_\\n]+__|`[^`\\n]+`|\\[[^\\]\\n]+\\]\\([^)\\n]+\\)|^[ \\t]*\\|.+\\|[ \\t]*$)")

func larkReplyPayload(agentName, content string) (string, string) {
	text := agentName + "：\n" + content
	if !strings.Contains(content, "```") && !larkMarkdownPattern.MatchString(content) {
		encoded, _ := json.Marshal(map[string]string{"text": text})
		return "text", string(encoded)
	}
	card, _ := json.Marshal(map[string]any{
		"schema": "2.0",
		"body": map[string]any{"elements": []any{
			map[string]any{"tag": "markdown", "content": agentName + "：\n\n" + content},
		}},
	})
	return "interactive", string(card)
}

func larkCallbackSignature(bindingID string) string {
	mac := hmac.New(sha256.New, auth.JWTSecret())
	_, _ = mac.Write([]byte("solo-lark-callback-v1:" + bindingID))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

func hashLarkToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func encryptLarkSecret(plain string) (string, error) {
	block, err := aes.NewCipher(larkEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptLarkSecret(encoded string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(larkEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted secret")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	return string(plain), err
}

func larkEncryptionKey() []byte {
	sum := sha256.Sum256(append([]byte("solo-lark-encryption-v1:"), auth.JWTSecret()...))
	return sum[:]
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
