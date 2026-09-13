package service

import (
	"context"
	"testing"
)

func TestAuthService_SavePushSubscription_Success(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "eto nikto ne prochitaet", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	if err := svc.SavePushSubscription(context.Background(), user.ID, "https://push.example.com/abc", "p256dh-key", "auth-key"); err != nil {
		t.Fatalf("SavePushSubscription() unexpected error: %v", err)
	}

	subs, err := svc.ListPushSubscriptions(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListPushSubscriptions() unexpected error: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("ListPushSubscriptions() returned %d subscriptions, want 1", len(subs))
	}
	if subs[0].Endpoint != "https://push.example.com/abc" || subs[0].P256dhKey != "p256dh-key" || subs[0].AuthKey != "auth-key" {
		t.Errorf("ListPushSubscriptions() = %+v, unexpected fields", subs[0])
	}
}

func TestAuthService_SavePushSubscription_Upsert(t *testing.T) {
	repo := newFakeUserRepository()
	svc := newTestAuthService(repo)

	user, err := svc.Register(context.Background(), "user@example.com", "balbes", "eto nikto ne prochitaet", "abcd1234", "test-public-key", "test-wrapped-key", "test-salt")
	if err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	endpoint := "https://push.example.com/abc"
	if err := svc.SavePushSubscription(context.Background(), user.ID, endpoint, "old-p256dh", "old-auth"); err != nil {
		t.Fatalf("SavePushSubscription() unexpected error: %v", err)
	}
	if err := svc.SavePushSubscription(context.Background(), user.ID, endpoint, "new-p256dh", "new-auth"); err != nil {
		t.Fatalf("SavePushSubscription() second call unexpected error: %v", err)
	}

	subs, err := svc.ListPushSubscriptions(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListPushSubscriptions() unexpected error: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("ListPushSubscriptions() returned %d subscriptions, want 1 (upsert should not duplicate)", len(subs))
	}
	if subs[0].P256dhKey != "new-p256dh" {
		t.Errorf("ListPushSubscriptions() P256dhKey = %q, want %q (upsert should overwrite)", subs[0].P256dhKey, "new-p256dh")
	}
}
