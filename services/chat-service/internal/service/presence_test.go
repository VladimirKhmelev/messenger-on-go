package service

import (
	"context"
	"testing"
)

func TestChatService_GetPresence(t *testing.T) {
	repo := newFakeChatRepository()
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), newFakePresenceChecker("user-a"), newFakeRateLimiter())

	online, _, err := svc.GetPresence(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("GetPresence() unexpected error: %v", err)
	}
	if !online {
		t.Error("GetPresence() online = false for an online user, want true")
	}

	online, _, err = svc.GetPresence(context.Background(), "user-b")
	if err != nil {
		t.Fatalf("GetPresence() unexpected error: %v", err)
	}
	if online {
		t.Error("GetPresence() online = true for an offline user, want false")
	}
}

func TestChatService_SetOffline(t *testing.T) {
	repo := newFakeChatRepository()
	presence := newFakePresenceChecker("user-a")
	svc := NewChatService(repo, newFakeAuthClient(), newFakeEventPublisher(), presence, newFakeRateLimiter())

	if err := svc.SetOffline(context.Background(), "user-a"); err != nil {
		t.Fatalf("SetOffline() unexpected error: %v", err)
	}

	online, _, err := svc.GetPresence(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("GetPresence() unexpected error: %v", err)
	}
	if online {
		t.Error("GetPresence() online = true after SetOffline, want false")
	}
}
