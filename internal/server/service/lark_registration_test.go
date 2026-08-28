package service

import (
	"slices"
	"testing"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestLarkInboundFromSDK(t *testing.T) {
	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{Header: &larkevent.EventHeader{EventID: "event-1"}},
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{SenderId: &larkim.UserId{OpenId: stringPointer("user-1")}},
			Message: &larkim.EventMessage{
				MessageId: stringPointer("message-1"), ChatId: stringPointer("chat-1"),
				ChatType: stringPointer("group"), MessageType: stringPointer("text"),
				Content:  stringPointer(`{"text":"@_user_1 hello"}`),
				Mentions: []*larkim.MentionEvent{{Key: stringPointer("@_user_1")}},
			},
		},
	}
	inbound, ok := larkInboundFromSDK(event)
	if !ok || inbound.Text != "hello" || !inbound.Mentioned || inbound.SenderOpenID != "user-1" {
		t.Fatalf("unexpected inbound event: %#v, ok=%v", inbound, ok)
	}
}

func TestLarkRegistrationUsesDefaultTemplateAndUpdatesExistingApp(t *testing.T) {
	created := newLarkRegistrationOptions("Lucy", "")
	if !created.CreateOnly || created.AppID != "" || created.Addons.Preset != nil {
		t.Fatalf("unexpected create options: %#v", created)
	}
	if !slices.Contains(created.Addons.Scopes.Tenant, "contact:user.base:readonly") {
		t.Fatalf("missing external member profile permission: %#v", created.Addons.Scopes.Tenant)
	}

	updated := newLarkRegistrationOptions("Lucy", "cli_existing")
	if updated.CreateOnly || updated.AppID != "cli_existing" || updated.Addons.Preset != nil {
		t.Fatalf("unexpected update options: %#v", updated)
	}
}

func stringPointer(value string) *string { return &value }
