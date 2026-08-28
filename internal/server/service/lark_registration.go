package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type LarkRegistration struct {
	ID          string    `json:"session_id"`
	Status      string    `json:"status"`
	QRCodeURL   string    `json:"qr_code_url,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	Error       string    `json:"error,omitempty"`
	WorkspaceID string    `json:"-"`
	Platform    string    `json:"-"`
	ChannelID   string    `json:"-"`
	AgentID     string    `json:"-"`
	UserID      string    `json:"-"`
}

type StartLarkRegistrationInput struct {
	WorkspaceID string
	Platform    string
	ChannelID   string
	AgentID     string
	UserID      string
}

type larkQRState struct {
	ctx         context.Context
	mu          sync.Mutex
	sessions    map[string]LarkRegistration
	connections map[string]*larkws.Client
}

func newLarkQRState(ctx context.Context) *larkQRState {
	return &larkQRState{ctx: ctx, sessions: make(map[string]LarkRegistration), connections: make(map[string]*larkws.Client)}
}

func (s *LarkService) StartRegistration(ctx context.Context, in StartLarkRegistrationInput) (*LarkRegistration, error) {
	if in.Platform != "feishu" && in.Platform != "lark" {
		return nil, errors.New("platform must be feishu or lark")
	}
	var agentName string
	err := s.pool.QueryRow(ctx, `
		SELECT a.name FROM channels c
		JOIN channel_members cm ON cm.channel_id = c.id AND cm.member_type = 'agent'
		JOIN agents a ON a.id = cm.member_id AND a.is_active = true
		WHERE c.id = $2 AND c.workspace_id = $1 AND a.id = $3`,
		in.WorkspaceID, in.ChannelID, in.AgentID).Scan(&agentName)
	if err != nil {
		return nil, errors.New("channel and Agent must belong to this Workspace")
	}
	existingAppID := ""
	existingBinding, err := s.GetBinding(ctx, in.WorkspaceID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if existingBinding != nil && existingBinding.ConnectionMode == "websocket" {
		existingAppID = existingBinding.AppID
	}

	session := LarkRegistration{
		ID: uuid.New().String(), Status: "starting", WorkspaceID: in.WorkspaceID,
		Platform: in.Platform, ChannelID: in.ChannelID, AgentID: in.AgentID, UserID: in.UserID,
	}
	s.qr.mu.Lock()
	s.qr.sessions[session.ID] = session
	s.qr.mu.Unlock()

	ready := make(chan struct{}, 1)
	go s.runRegistration(session.ID, agentName, existingAppID, ready)
	select {
	case <-ready:
		return s.RegistrationStatus(in.WorkspaceID, session.ID)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(15 * time.Second):
		return s.RegistrationStatus(in.WorkspaceID, session.ID)
	}
}

func (s *LarkService) RegistrationStatus(workspaceID, sessionID string) (*LarkRegistration, error) {
	s.qr.mu.Lock()
	defer s.qr.mu.Unlock()
	session, ok := s.qr.sessions[sessionID]
	if !ok || session.WorkspaceID != workspaceID {
		return nil, errors.New("registration not found")
	}
	if session.Status == "waiting" && !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		session.Status = "expired"
		s.qr.sessions[sessionID] = session
	}
	return &session, nil
}

func (s *LarkService) runRegistration(sessionID, agentName, existingAppID string, ready chan<- struct{}) {
	session, err := s.registrationStatusByID(sessionID)
	if err != nil {
		return
	}
	options := newLarkRegistrationOptions(agentName, existingAppID)
	options.OnQRCode = func(info *registration.QRCodeInfo) {
		s.updateRegistration(sessionID, func(value *LarkRegistration) {
			value.Status = "waiting"
			value.QRCodeURL = info.URL
			value.ExpiresAt = time.Now().Add(time.Duration(info.ExpireIn) * time.Second)
		})
		select {
		case ready <- struct{}{}:
		default:
		}
	}
	if session.Platform == "lark" {
		options.Domain = "https://accounts.larksuite.com"
		options.LarkDomain = options.Domain
	}

	registrationCtx, cancel := context.WithTimeout(s.qr.ctx, 11*time.Minute)
	defer cancel()
	result, err := registration.RegisterApp(registrationCtx, options)
	if err != nil {
		s.updateRegistration(sessionID, func(value *LarkRegistration) {
			value.Status = "error"
			value.Error = err.Error()
		})
		select {
		case ready <- struct{}{}:
		default:
		}
		return
	}
	platform := session.Platform
	if result.UserInfo != nil && (result.UserInfo.TenantBrand == "feishu" || result.UserInfo.TenantBrand == "lark") {
		platform = result.UserInfo.TenantBrand
	}
	binding, err := s.saveWebsocketBinding(context.Background(), session, platform, result.ClientID, result.ClientSecret)
	if err != nil {
		s.updateRegistration(sessionID, func(value *LarkRegistration) {
			value.Status = "error"
			value.Error = err.Error()
		})
		return
	}
	s.updateRegistration(sessionID, func(value *LarkRegistration) { value.Status = "connected" })
	s.startLarkWebsocket(binding.ID)
}

func newLarkRegistrationOptions(agentName, existingAppID string) *registration.Options {
	return &registration.Options{
		Source: "solo", AppID: existingAppID, CreateOnly: existingAppID == "",
		AppPreset: &registration.AppPreset{Name: agentName + " · Solo", Desc: "Connect this bot to Solo."},
		Addons: &registration.AppAddons{
			Scopes: registration.AppAddonsScopes{Tenant: []string{"im:message:send_as_bot", "contact:user.base:readonly"}},
			Events: registration.AppAddonsEvents{Items: registration.AppAddonsEventItems{Tenant: []string{"im.message.receive_v1"}}},
		},
	}
}

func (s *LarkService) registrationStatusByID(sessionID string) (LarkRegistration, error) {
	s.qr.mu.Lock()
	defer s.qr.mu.Unlock()
	session, ok := s.qr.sessions[sessionID]
	if !ok {
		return LarkRegistration{}, errors.New("registration not found")
	}
	return session, nil
}

func (s *LarkService) updateRegistration(sessionID string, update func(*LarkRegistration)) {
	s.qr.mu.Lock()
	defer s.qr.mu.Unlock()
	session, ok := s.qr.sessions[sessionID]
	if !ok {
		return
	}
	update(&session)
	s.qr.sessions[sessionID] = session
}

func (s *LarkService) saveWebsocketBinding(ctx context.Context, session LarkRegistration, platform, appID, appSecret string) (*LarkBinding, error) {
	encryptedSecret, err := encryptLarkSecret(appSecret)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO lark_bindings (
		  workspace_id, channel_id, agent_id, platform, app_id,
		  app_secret_encrypted, verification_token_hash, created_by,
		  connection_mode, connection_status, connection_error
		) VALUES ($1,$2,$3,$4,$5,$6,'',$7,'websocket','connecting',NULL)
		ON CONFLICT (workspace_id) DO UPDATE SET
		  channel_id=EXCLUDED.channel_id, agent_id=EXCLUDED.agent_id, platform=EXCLUDED.platform,
		  app_id=EXCLUDED.app_id, app_secret_encrypted=EXCLUDED.app_secret_encrypted,
		  verification_token_hash='', external_chat_id=NULL, external_chat_type=NULL,
		  connection_mode='websocket', connection_status='connecting', connection_error=NULL,
		  updated_at=now()`,
		session.WorkspaceID, session.ChannelID, session.AgentID, platform, appID, encryptedSecret, session.UserID)
	if err != nil {
		return nil, err
	}
	return s.GetBinding(ctx, session.WorkspaceID)
}

func (s *LarkService) restoreLarkWebsocketBindings(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM lark_bindings WHERE connection_mode='websocket'`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var bindingID string
		if rows.Scan(&bindingID) == nil {
			s.startLarkWebsocket(bindingID)
		}
	}
}

func (s *LarkService) startLarkWebsocket(bindingID string) {
	var workspaceID, platform, appID, encryptedSecret string
	err := s.pool.QueryRow(context.Background(), `
		SELECT workspace_id, platform, app_id, app_secret_encrypted
		FROM lark_bindings WHERE id=$1 AND connection_mode='websocket'`, bindingID).
		Scan(&workspaceID, &platform, &appID, &encryptedSecret)
	if err != nil {
		return
	}
	secret, err := decryptLarkSecret(encryptedSecret)
	if err != nil {
		s.setLarkConnectionStatus(bindingID, "error", err.Error())
		return
	}

	handler := dispatcher.NewEventDispatcher("", "").OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		inbound, ok := larkInboundFromSDK(event)
		if !ok {
			return nil
		}
		binding, err := s.GetBinding(ctx, workspaceID)
		if err != nil {
			return err
		}
		return s.HandleInbound(ctx, binding, inbound)
	})
	domain := lark.FeishuBaseUrl
	if platform == "lark" {
		domain = lark.LarkBaseUrl
	}
	client := larkws.NewClient(appID, secret,
		larkws.WithEventHandler(handler), larkws.WithDomain(domain), larkws.WithAutoReconnect(true),
		larkws.WithOnReady(func() { s.setLarkConnectionStatus(bindingID, "connected", "") }),
		larkws.WithOnReconnected(func() { s.setLarkConnectionStatus(bindingID, "connected", "") }),
		larkws.WithOnReconnecting(func() { s.setLarkConnectionStatus(bindingID, "connecting", "") }),
		larkws.WithOnError(func(err error) { s.setLarkConnectionStatus(bindingID, "error", err.Error()) }),
	)
	s.stopLarkWebsocket(bindingID)
	s.qr.mu.Lock()
	s.qr.connections[bindingID] = client
	s.qr.mu.Unlock()
	go func() {
		if err := client.Start(s.qr.ctx); err != nil {
			s.setLarkConnectionStatus(bindingID, "error", err.Error())
		}
	}()
}

func (s *LarkService) stopLarkWebsocket(bindingID string) {
	s.qr.mu.Lock()
	client := s.qr.connections[bindingID]
	delete(s.qr.connections, bindingID)
	s.qr.mu.Unlock()
	if client != nil {
		client.Close()
	}
}

func (s *LarkService) setLarkConnectionStatus(bindingID, status, message string) {
	_, _ = s.pool.Exec(context.Background(), `
		UPDATE lark_bindings SET connection_status=$2, connection_error=NULLIF($3,''), updated_at=now()
		WHERE id=$1`, bindingID, status, message)
}

func larkInboundFromSDK(event *larkim.P2MessageReceiveV1) (LarkInboundEvent, bool) {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil ||
		event.Event.Sender.SenderId == nil || value(event.Event.Message.MessageType) != "text" {
		return LarkInboundEvent{}, false
	}
	var content struct {
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(value(event.Event.Message.Content)), &content) != nil {
		return LarkInboundEvent{}, false
	}
	for _, mention := range event.Event.Message.Mentions {
		if mention != nil {
			content.Text = strings.ReplaceAll(content.Text, value(mention.Key), "")
		}
	}
	eventID := ""
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		eventID = event.EventV2Base.Header.EventID
	}
	return LarkInboundEvent{
		EventID: eventID, MessageID: value(event.Event.Message.MessageId),
		ChatID: value(event.Event.Message.ChatId), ChatType: value(event.Event.Message.ChatType),
		SenderOpenID: value(event.Event.Sender.SenderId.OpenId), Text: strings.TrimSpace(content.Text),
		Mentioned: len(event.Event.Message.Mentions) > 0,
	}, true
}

func value(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
