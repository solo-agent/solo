package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PushService manages WebPush subscription storage and notification delivery
// using the Web Push protocol (RFC 8291) with VAPID authentication.
type PushService struct {
	pool       *pgxpool.Pool
	vapidKeys  VAPIDKeys
	httpClient *http.Client
}

// VAPIDKeys holds the Voluntary Application Server Identification keys.
type VAPIDKeys struct {
	Subject    string // "mailto:admin@example.com" or app URL
	PublicKey  string
	PrivateKey string
}

// PushSubscription represents a stored browser push subscription.
type PushSubscription struct {
	UserID   string
	Endpoint string
	P256DH   string
	Auth     string
}

// NewPushService creates a new PushService.
func NewPushService(pool *pgxpool.Pool, vapidKeys VAPIDKeys) *PushService {
	return &PushService{
		pool:       pool,
		vapidKeys:  vapidKeys,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GenerateVAPIDKeys creates a new VAPID key pair (P-256 ECDSA).
func GenerateVAPIDKeys(subject string) (*VAPIDKeys, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ECDSA key: %w", err)
	}

	publicKey := elliptic.Marshal(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	privateKeyBytes := privateKey.D.Bytes()

	return &VAPIDKeys{
		Subject:    subject,
		PublicKey:  base64.RawURLEncoding.EncodeToString(publicKey),
		PrivateKey: base64.RawURLEncoding.EncodeToString(privateKeyBytes),
	}, nil
}

// Subscribe stores a push subscription for a user.
func (s *PushService) Subscribe(ctx context.Context, userID, endpoint, p256dh, auth string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, endpoint) DO UPDATE SET p256dh = $3, auth = $4, created_at = now()`,
		userID, endpoint, p256dh, auth,
	)
	return err
}

// Unsubscribe removes a push subscription.
func (s *PushService) Unsubscribe(ctx context.Context, userID, endpoint string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`,
		userID, endpoint,
	)
	return err
}

// NotifyRunFinished sends push notifications to all subscribers when an agent run completes.
func (s *PushService) NotifyRunFinished(ctx context.Context, runnerID, agentName, channelID string, status string) {
	rows, err := s.pool.Query(ctx,
		`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = $1`,
		runnerID,
	)
	if err != nil {
		slog.Warn("push: failed to query subscriptions", "error", err)
		return
	}
	defer rows.Close()

	title := fmt.Sprintf("Agent %s finished", agentName)
	body := fmt.Sprintf("Task completed with status: %s", status)

	var count int
	for rows.Next() {
		var sub PushSubscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256DH, &sub.Auth); err != nil {
			continue
		}
		if err := s.sendPushNotification(ctx, sub, title, body, channelID); err != nil {
			slog.Warn("push: failed to send notification", "endpoint", sub.Endpoint, "error", err)
			continue
		}
		count++
	}
	if count > 0 {
		slog.Info("push: sent notifications", "count", count, "agent", agentName)
	}
}

// sendPushNotification delivers a single Web Push notification using VAPID.
// This implementation sends a tile-only notification (no encrypted payload).
// For full payload support per RFC 8291 §3, add aes128gcm encryption using
// the subscriber's p256dh key.
func (s *PushService) sendPushNotification(ctx context.Context, sub PushSubscription, title, body, channelID string) error {
	vapidToken, err := s.createVAPIDJWT(sub.Endpoint)
	if err != nil {
		return fmt.Errorf("vapid jwt: %w", err)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"title": title,
		"body":  body,
		"url":   "/channels/" + channelID,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "vapid t="+vapidToken+", k="+s.vapidKeys.PublicKey)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "86400")

	_ = payload // for future encrypted payload support
	_ = sub     // for future p256dh decryption

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != 404 && resp.StatusCode != 410 {
		return fmt.Errorf("push endpoint returned %d", resp.StatusCode)
	}
	return nil
}

// createVAPIDJWT creates a signed JWT for VAPID authentication.
func (s *PushService) createVAPIDJWT(audience string) (string, error) {
	privateKey, err := s.decodeVAPIDPrivateKey()
	if err != nil {
		return "", err
	}

	claims := jwt.MapClaims{
		"aud": audience,
		"exp": time.Now().Add(12 * time.Hour).Unix(),
		"sub": s.vapidKeys.Subject,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("sign vapid jwt: %w", err)
	}
	return tokenString, nil
}

// decodeVAPIDPrivateKey reconstructs an ECDSA P-256 private key from the raw
// base64-encoded private scalar stored in VAPIDKeys.
func (s *PushService) decodeVAPIDPrivateKey() (*ecdsa.PrivateKey, error) {
	privateKeyBytes, err := base64.RawURLEncoding.DecodeString(s.vapidKeys.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}

	curve := elliptic.P256()
	privateKey := new(ecdsa.PrivateKey)
	privateKey.D = new(big.Int).SetBytes(privateKeyBytes)
	privateKey.PublicKey.Curve = curve
	privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(privateKeyBytes)

	return privateKey, nil
}
