package service

import (
	"encoding/json"
	"testing"
)

func TestLarkReplyPayload(t *testing.T) {
	messageType, content := larkReplyPayload("Lucy", "你好，请说。")
	if messageType != "text" {
		t.Fatalf("plain reply type = %q, want text", messageType)
	}
	var textPayload map[string]string
	if err := json.Unmarshal([]byte(content), &textPayload); err != nil || textPayload["text"] != "Lucy：\n你好，请说。" {
		t.Fatalf("plain reply payload = %q, err = %v", content, err)
	}

	markdown := "# 总结\n\n- **完成** 第一项\n\n```go\nfmt.Println(\"ok\")\n```"
	messageType, content = larkReplyPayload("Lucy", markdown)
	if messageType != "interactive" {
		t.Fatalf("markdown reply type = %q, want interactive", messageType)
	}
	var card struct {
		Schema string `json:"schema"`
		Body   struct {
			Elements []struct {
				Tag     string `json:"tag"`
				Content string `json:"content"`
			} `json:"elements"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(content), &card); err != nil {
		t.Fatalf("decode markdown card: %v", err)
	}
	if card.Schema != "2.0" || len(card.Body.Elements) != 1 || card.Body.Elements[0].Tag != "markdown" || card.Body.Elements[0].Content != "Lucy：\n\n"+markdown {
		t.Fatalf("unexpected markdown card: %+v", card)
	}
	if messageType, _ := larkReplyPayload("Lucy", "请运行 `make rebuild`"); messageType != "interactive" {
		t.Fatalf("inline code reply type = %q, want interactive", messageType)
	}
}
