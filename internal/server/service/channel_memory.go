package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ChannelMemoryService manages shared channel memory files
// stored under ~/.solo/channels/<channelID>/memory/.
type ChannelMemoryService struct {
	basePath string
	mu       sync.RWMutex
}

func NewChannelMemoryService(basePath string) *ChannelMemoryService {
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		basePath = filepath.Join(home, ".solo", "channels")
	}
	return &ChannelMemoryService{basePath: basePath}
}

// channelMemoryPath returns the memory directory for a channel.
func (s *ChannelMemoryService) channelMemoryPath(channelID string) string {
	return filepath.Join(s.basePath, channelID, "memory")
}

// ReadCHANNEL reads the shared CHANNEL.md for a channel.
func (s *ChannelMemoryService) ReadCHANNEL(channelID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path := filepath.Join(s.channelMemoryPath(channelID), "CHANNEL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read CHANNEL.md: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteCHANNEL overwrites CHANNEL.md for a channel.
func (s *ChannelMemoryService) WriteCHANNEL(channelID, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.channelMemoryPath(channelID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create channel memory dir: %w", err)
	}
	path := filepath.Join(dir, "CHANNEL.md")
	return os.WriteFile(path, []byte(content), 0o644)
}

// AppendDecision appends a decision entry to decisions.md.
func (s *ChannelMemoryService) AppendDecision(channelID, agentName, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.channelMemoryPath(channelID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create channel memory dir: %w", err)
	}
	path := filepath.Join(dir, "decisions.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open decisions.md: %w", err)
	}
	defer f.Close()
	entry := fmt.Sprintf("- [%s] @%s: %s\n", time.Now().UTC().Format(time.RFC3339), agentName, content)
	_, err = f.WriteString(entry)
	return err
}

// ReadDecisions reads the full decisions.md for a channel.
func (s *ChannelMemoryService) ReadDecisions(channelID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path := filepath.Join(s.channelMemoryPath(channelID), "decisions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read decisions.md: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// EnsureChannelMemory creates the memory directory structure for a channel.
func (s *ChannelMemoryService) EnsureChannelMemory(channelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.channelMemoryPath(channelID)
	return os.MkdirAll(dir, 0o755)
}
