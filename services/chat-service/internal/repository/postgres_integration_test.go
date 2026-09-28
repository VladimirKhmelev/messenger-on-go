//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

// One container for the whole package: every test works with its own random
// UUIDs, so tests never see each other's rows.
var testRepo *PostgresChatRepository

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("chat_test"),
		postgres.WithUsername("chat_test"),
		postgres.WithPassword("chat_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	code := func() int {
		defer func() { _ = container.Terminate(ctx) }()

		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
			return 1
		}
		testRepo, err = NewPostgresChatRepository(dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect repository: %v\n", err)
			return 1
		}
		if err := testRepo.Migrate(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to run migrations: %v\n", err)
			return 1
		}
		return m.Run()
	}()
	os.Exit(code)
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func key(userID string) domain.MemberChatKey {
	return domain.MemberChatKey{EncryptedChatKey: "enc-" + userID, WrappedForPublicKey: "pub-" + userID}
}

func createPrivateChat(t *testing.T, createdAt time.Time, userA, userB string) *domain.Chat {
	t.Helper()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: createdAt, ChatType: domain.ChatTypePrivate}
	keys := map[string]domain.MemberChatKey{userA: key(userA), userB: key(userB)}
	if err := testRepo.CreateChat(context.Background(), chat, keys); err != nil {
		t.Fatalf("CreateChat() error: %v", err)
	}
	return chat
}

func createGroupChat(t *testing.T, creatorID string, memberIDs ...string) *domain.Chat {
	t.Helper()
	name := "group"
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: now(), ChatType: domain.ChatTypeGroup, Name: &name, CreatedBy: &creatorID}
	keys := map[string]domain.MemberChatKey{creatorID: key(creatorID)}
	for _, id := range memberIDs {
		keys[id] = key(id)
	}
	if err := testRepo.CreateChat(context.Background(), chat, keys); err != nil {
		t.Fatalf("CreateChat() error: %v", err)
	}
	return chat
}

func createMessage(t *testing.T, chatID, senderID, body string, createdAt time.Time) *domain.Message {
	t.Helper()
	msg := &domain.Message{ID: uuid.NewString(), ChatID: chatID, SenderID: senderID, Body: body, CreatedAt: createdAt}
	if err := testRepo.CreateMessage(context.Background(), msg); err != nil {
		t.Fatalf("CreateMessage() error: %v", err)
	}
	return msg
}

func appendEvent(t *testing.T, msg *domain.Message, actorID string, typ domain.MessageEventType, newBody *string, at time.Time) {
	t.Helper()
	event := &domain.MessageEvent{
		ID: uuid.NewString(), MessageID: msg.ID, ChatID: msg.ChatID, ActorID: actorID,
		Type: typ, NewBody: newBody, CreatedAt: at,
	}
	if err := testRepo.AppendMessageEvent(context.Background(), event); err != nil {
		t.Fatalf("AppendMessageEvent() error: %v", err)
	}
}

func messageIDs(msgs []*domain.Message) []string {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids
}

func TestPostgresChatRepository_CreateAndGetChat(t *testing.T) {
	ctx := context.Background()
	alice, bob, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	created := now()
	chat := createPrivateChat(t, created, alice, bob)

	got, err := testRepo.GetChat(ctx, chat.ID)
	if err != nil {
		t.Fatalf("GetChat() error: %v", err)
	}
	if got.ChatType != domain.ChatTypePrivate || !got.CreatedAt.Equal(created) || got.Name != nil || got.CreatedBy != nil {
		t.Errorf("GetChat() = %+v, want private chat created at %v without name/creator", got, created)
	}

	for user, want := range map[string]bool{alice: true, bob: true, stranger: false} {
		if isMember, err := testRepo.IsMember(ctx, chat.ID, user); err != nil || isMember != want {
			t.Errorf("IsMember(%s) = %v, %v; want %v", user, isMember, err, want)
		}
	}
	if count, err := testRepo.MemberCount(ctx, chat.ID); err != nil || count != 2 {
		t.Errorf("MemberCount() = %d, %v; want 2", count, err)
	}

	if _, err := testRepo.GetChat(ctx, uuid.NewString()); !errors.Is(err, domain.ErrChatNotFound) {
		t.Errorf("GetChat(unknown) error = %v, want %v", err, domain.ErrChatNotFound)
	}
}

func TestPostgresChatRepository_FindPrivateChat(t *testing.T) {
	ctx := context.Background()
	alice, bob, carol := uuid.NewString(), uuid.NewString(), uuid.NewString()
	pair := createPrivateChat(t, now(), alice, bob)
	self := &domain.Chat{ID: uuid.NewString(), CreatedAt: now(), ChatType: domain.ChatTypePrivate}
	if err := testRepo.CreateChat(ctx, self, map[string]domain.MemberChatKey{alice: key(alice)}); err != nil {
		t.Fatalf("CreateChat(self) error: %v", err)
	}
	// a group with the same two people must not be mistaken for their private chat
	createGroupChat(t, alice, bob)

	for _, tc := range []struct {
		name       string
		userA      string
		userB      string
		wantChatID string
	}{
		{"pair", alice, bob, pair.ID},
		{"pair reversed", bob, alice, pair.ID},
		{"saved messages", alice, alice, self.ID},
	} {
		got, err := testRepo.FindPrivateChat(ctx, tc.userA, tc.userB)
		if err != nil || got.ID != tc.wantChatID {
			t.Errorf("%s: FindPrivateChat() = %v, %v; want chat %s", tc.name, got, err, tc.wantChatID)
		}
	}

	if _, err := testRepo.FindPrivateChat(ctx, alice, carol); !errors.Is(err, domain.ErrChatNotFound) {
		t.Errorf("FindPrivateChat(no chat) error = %v, want %v", err, domain.ErrChatNotFound)
	}
}

func TestPostgresChatRepository_GroupMembersAndRoles(t *testing.T) {
	ctx := context.Background()
	creator, bob, carol, dave := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	chat := createGroupChat(t, creator, bob, carol)

	if isAdmin, _ := testRepo.IsAdmin(ctx, chat.ID, creator); !isAdmin {
		t.Error("creator is not admin, want admin")
	}
	if isAdmin, _ := testRepo.IsAdmin(ctx, chat.ID, bob); isAdmin {
		t.Error("bob is admin, want member")
	}

	if err := testRepo.SetRole(ctx, chat.ID, bob, domain.MemberRoleAdmin); err != nil {
		t.Fatalf("SetRole() error: %v", err)
	}
	if member, err := testRepo.GetMember(ctx, chat.ID, bob); err != nil || member.Role != domain.MemberRoleAdmin {
		t.Errorf("GetMember(bob) = %+v, %v; want admin", member, err)
	}

	if err := testRepo.AddMember(ctx, chat.ID, dave, key(dave)); err != nil {
		t.Fatalf("AddMember() error: %v", err)
	}
	if member, err := testRepo.GetMember(ctx, chat.ID, dave); err != nil || member.Role != domain.MemberRoleMember || member.EncryptedChatKey != "enc-"+dave {
		t.Errorf("GetMember(dave) = %+v, %v; want member with his key", member, err)
	}

	if err := testRepo.RemoveMember(ctx, chat.ID, carol); err != nil {
		t.Fatalf("RemoveMember() error: %v", err)
	}
	members, err := testRepo.ListMembers(ctx, chat.ID)
	if err != nil || len(members) != 3 {
		t.Fatalf("ListMembers() = %d members, %v; want 3", len(members), err)
	}
	if members[len(members)-1].UserID != dave {
		t.Errorf("last member = %s, want dave (ordered by joined_at)", members[len(members)-1].UserID)
	}

	stranger := uuid.NewString()
	if err := testRepo.RemoveMember(ctx, chat.ID, stranger); !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("RemoveMember(stranger) error = %v, want %v", err, domain.ErrNotChatMember)
	}
	if err := testRepo.SetRole(ctx, chat.ID, stranger, domain.MemberRoleAdmin); !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("SetRole(stranger) error = %v, want %v", err, domain.ErrNotChatMember)
	}
	if _, err := testRepo.GetMember(ctx, chat.ID, stranger); !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("GetMember(stranger) error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestPostgresChatRepository_ChatKeys(t *testing.T) {
	ctx := context.Background()
	alice, bob := uuid.NewString(), uuid.NewString()
	chat := createPrivateChat(t, now(), alice, bob)

	if got, err := testRepo.GetChatKeyForUser(ctx, chat.ID, alice); err != nil || got != "enc-"+alice {
		t.Errorf("GetChatKeyForUser() = %q, %v; want alice's key", got, err)
	}

	if err := testRepo.UpdateChatKey(ctx, chat.ID, alice, "rekeyed", "new-pub"); err != nil {
		t.Fatalf("UpdateChatKey() error: %v", err)
	}
	member, err := testRepo.GetMember(ctx, chat.ID, alice)
	if err != nil || member.EncryptedChatKey != "rekeyed" || member.WrappedForPublicKey != "new-pub" {
		t.Errorf("after UpdateChatKey: %+v, %v", member, err)
	}
	if got, _ := testRepo.GetChatKeyForUser(ctx, chat.ID, bob); got != "enc-"+bob {
		t.Errorf("bob's key changed to %q, want untouched", got)
	}

	stranger := uuid.NewString()
	if _, err := testRepo.GetChatKeyForUser(ctx, chat.ID, stranger); !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("GetChatKeyForUser(stranger) error = %v, want %v", err, domain.ErrNotChatMember)
	}
	if err := testRepo.UpdateChatKey(ctx, chat.ID, stranger, "x", "y"); !errors.Is(err, domain.ErrNotChatMember) {
		t.Errorf("UpdateChatKey(stranger) error = %v, want %v", err, domain.ErrNotChatMember)
	}
}

func TestPostgresChatRepository_MessageEditAndDeleteProjection(t *testing.T) {
	ctx := context.Background()
	alice, bob := uuid.NewString(), uuid.NewString()
	chat := createPrivateChat(t, now(), alice, bob)
	sentAt := now()
	msg := createMessage(t, chat.ID, alice, "original", sentAt)

	got, err := testRepo.GetMessage(ctx, msg.ID)
	if err != nil || got.Body != "original" || got.EditedAt != nil || got.DeletedAt != nil || !got.CreatedAt.Equal(sentAt) {
		t.Fatalf("GetMessage() = %+v, %v; want unedited original", got, err)
	}

	first, second := "first edit", "second edit"
	appendEvent(t, msg, alice, domain.MessageEventEdited, &first, sentAt.Add(time.Second))
	secondAt := sentAt.Add(2 * time.Second)
	appendEvent(t, msg, alice, domain.MessageEventEdited, &second, secondAt)

	got, _ = testRepo.GetMessage(ctx, msg.ID)
	if got.Body != "second edit" || got.EditedAt == nil || !got.EditedAt.Equal(secondAt) {
		t.Errorf("after two edits: body=%q editedAt=%v; want latest edit at %v", got.Body, got.EditedAt, secondAt)
	}

	deletedAt := sentAt.Add(3 * time.Second)
	appendEvent(t, msg, alice, domain.MessageEventDeletedForAll, nil, deletedAt)
	got, _ = testRepo.GetMessage(ctx, msg.ID)
	if got.DeletedAt == nil || !got.DeletedAt.Equal(deletedAt) {
		t.Errorf("after delete: deletedAt=%v, want %v", got.DeletedAt, deletedAt)
	}

	if _, err := testRepo.GetMessage(ctx, uuid.NewString()); !errors.Is(err, domain.ErrMessageNotFound) {
		t.Errorf("GetMessage(unknown) error = %v, want %v", err, domain.ErrMessageNotFound)
	}
}

func TestPostgresChatRepository_ListMessagesPaginationAndHidden(t *testing.T) {
	ctx := context.Background()
	alice, bob := uuid.NewString(), uuid.NewString()
	chat := createPrivateChat(t, now(), alice, bob)

	if last, err := testRepo.GetLastMessage(ctx, chat.ID, alice); err != nil || last != nil {
		t.Fatalf("GetLastMessage(empty chat) = %v, %v; want nil, nil", last, err)
	}

	base := now()
	var msgs []*domain.Message
	for i := 0; i < 5; i++ {
		msgs = append(msgs, createMessage(t, chat.ID, alice, fmt.Sprintf("m%d", i), base.Add(time.Duration(i)*time.Second)))
	}

	// newest page first, but each page is returned oldest → newest
	page1, err := testRepo.ListMessages(ctx, chat.ID, alice, 2, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if want := messageIDs(msgs[3:5]); fmt.Sprint(messageIDs(page1)) != fmt.Sprint(want) {
		t.Errorf("page 1 = %v, want %v", messageIDs(page1), want)
	}
	page2, _ := testRepo.ListMessages(ctx, chat.ID, alice, 2, 2)
	if want := messageIDs(msgs[1:3]); fmt.Sprint(messageIDs(page2)) != fmt.Sprint(want) {
		t.Errorf("page 2 = %v, want %v", messageIDs(page2), want)
	}

	// hiding is per user and idempotent
	newest := msgs[4]
	for i := 0; i < 2; i++ {
		if err := testRepo.HideMessageForUser(ctx, newest.ID, alice); err != nil {
			t.Fatalf("HideMessageForUser() attempt %d error: %v", i+1, err)
		}
	}
	forAlice, _ := testRepo.ListMessages(ctx, chat.ID, alice, 10, 0)
	forBob, _ := testRepo.ListMessages(ctx, chat.ID, bob, 10, 0)
	if len(forAlice) != 4 || len(forBob) != 5 {
		t.Errorf("after hiding for alice: alice sees %d, bob sees %d; want 4 and 5", len(forAlice), len(forBob))
	}

	if last, _ := testRepo.GetLastMessage(ctx, chat.ID, alice); last == nil || last.ID != msgs[3].ID {
		t.Errorf("GetLastMessage(alice) = %v, want m3 (m4 is hidden for her)", last)
	}
	if last, _ := testRepo.GetLastMessage(ctx, chat.ID, bob); last == nil || last.ID != newest.ID {
		t.Errorf("GetLastMessage(bob) = %v, want m4", last)
	}
}

func TestPostgresChatRepository_ListChatsForUserOrderedByActivity(t *testing.T) {
	ctx := context.Background()
	alice, bob, carol := uuid.NewString(), uuid.NewString(), uuid.NewString()
	base := now()
	older := createPrivateChat(t, base, alice, bob)
	newer := createPrivateChat(t, base.Add(10*time.Second), alice, carol)

	chats, err := testRepo.ListChatsForUser(ctx, alice)
	if err != nil || len(chats) != 2 || chats[0].ID != newer.ID {
		t.Fatalf("without messages: %v, %v; want newer chat first", chats, err)
	}

	// a fresh message lifts the older chat to the top
	createMessage(t, older.ID, bob, "ping", base.Add(20*time.Second))
	chats, _ = testRepo.ListChatsForUser(ctx, alice)
	if len(chats) != 2 || chats[0].ID != older.ID {
		t.Errorf("after message in older chat: first = %v, want older chat", chats[0].ID)
	}

	if chats, _ := testRepo.ListChatsForUser(ctx, bob); len(chats) != 1 {
		t.Errorf("bob sees %d chats, want 1", len(chats))
	}
}

func TestPostgresChatRepository_MarkReadAndDeleteChatCascade(t *testing.T) {
	ctx := context.Background()
	alice, bob := uuid.NewString(), uuid.NewString()
	chat := createGroupChat(t, alice, bob)
	msg := createMessage(t, chat.ID, alice, "hi", now())

	readAt := now()
	if err := testRepo.MarkRead(ctx, chat.ID, bob, msg.ID, readAt); err != nil {
		t.Fatalf("MarkRead() error: %v", err)
	}
	member, err := testRepo.GetMember(ctx, chat.ID, bob)
	if err != nil || member.LastReadMessageID == nil || *member.LastReadMessageID != msg.ID || !member.LastReadAt.Equal(readAt) {
		t.Fatalf("after MarkRead: %+v, %v", member, err)
	}

	if err := testRepo.UpsertChatAvatar(ctx, &domain.ChatAvatar{ChatID: chat.ID, Data: []byte("png"), ContentType: "image/png", UpdatedAt: now()}); err != nil {
		t.Fatalf("UpsertChatAvatar() error: %v", err)
	}

	if err := testRepo.DeleteChat(ctx, chat.ID); err != nil {
		t.Fatalf("DeleteChat() error: %v", err)
	}
	if _, err := testRepo.GetChat(ctx, chat.ID); !errors.Is(err, domain.ErrChatNotFound) {
		t.Errorf("GetChat() after delete error = %v, want %v", err, domain.ErrChatNotFound)
	}
	if _, err := testRepo.GetMessage(ctx, msg.ID); !errors.Is(err, domain.ErrMessageNotFound) {
		t.Errorf("GetMessage() after delete error = %v, want %v", err, domain.ErrMessageNotFound)
	}
	if count, _ := testRepo.MemberCount(ctx, chat.ID); count != 0 {
		t.Errorf("MemberCount() after delete = %d, want 0", count)
	}
	if _, err := testRepo.GetChatAvatar(ctx, chat.ID); !errors.Is(err, domain.ErrGroupAvatarNotFound) {
		t.Errorf("GetChatAvatar() after delete error = %v, want %v", err, domain.ErrGroupAvatarNotFound)
	}
}

func TestPostgresChatRepository_UpsertChatAvatar(t *testing.T) {
	ctx := context.Background()
	chat := createGroupChat(t, uuid.NewString())

	if _, err := testRepo.GetChatAvatar(ctx, chat.ID); !errors.Is(err, domain.ErrGroupAvatarNotFound) {
		t.Fatalf("GetChatAvatar(none) error = %v, want %v", err, domain.ErrGroupAvatarNotFound)
	}

	first := &domain.ChatAvatar{ChatID: chat.ID, Data: []byte("png-1"), ContentType: "image/png", UpdatedAt: now()}
	second := &domain.ChatAvatar{ChatID: chat.ID, Data: []byte("jpeg-2"), ContentType: "image/jpeg", UpdatedAt: now().Add(time.Second)}
	for _, a := range []*domain.ChatAvatar{first, second} {
		if err := testRepo.UpsertChatAvatar(ctx, a); err != nil {
			t.Fatalf("UpsertChatAvatar() error: %v", err)
		}
	}

	got, err := testRepo.GetChatAvatar(ctx, chat.ID)
	if err != nil || string(got.Data) != "jpeg-2" || got.ContentType != "image/jpeg" || !got.UpdatedAt.Equal(second.UpdatedAt) {
		t.Errorf("GetChatAvatar() = %+v, %v; want the second upload", got, err)
	}
}

func TestPostgresChatRepository_BlockUsers(t *testing.T) {
	ctx := context.Background()
	alice, bob, carol := uuid.NewString(), uuid.NewString(), uuid.NewString()

	for i := 0; i < 2; i++ {
		if err := testRepo.BlockUser(ctx, alice, bob); err != nil {
			t.Fatalf("BlockUser() attempt %d error: %v", i+1, err)
		}
	}

	for _, pair := range [][2]string{{alice, bob}, {bob, alice}} {
		if blocked, err := testRepo.IsBlocked(ctx, pair[0], pair[1]); err != nil || !blocked {
			t.Errorf("IsBlocked(%s, %s) = %v, %v; want true", pair[0], pair[1], blocked, err)
		}
	}
	if blocked, _ := testRepo.IsBlocked(ctx, alice, carol); blocked {
		t.Error("IsBlocked(alice, carol) = true, want false")
	}

	list, err := testRepo.ListBlockedUsers(ctx, alice)
	if err != nil || len(list) != 1 || list[0].BlockedID != bob {
		t.Errorf("ListBlockedUsers(alice) = %+v, %v; want only bob", list, err)
	}
	if list, _ := testRepo.ListBlockedUsers(ctx, bob); len(list) != 0 {
		t.Errorf("ListBlockedUsers(bob) = %+v, want empty: the block is one-directional in storage", list)
	}

	if err := testRepo.UnblockUser(ctx, alice, bob); err != nil {
		t.Fatalf("UnblockUser() error: %v", err)
	}
	if blocked, _ := testRepo.IsBlocked(ctx, alice, bob); blocked {
		t.Error("IsBlocked() after unblock = true, want false")
	}
}

func TestPostgresChatRepository_MessageReports(t *testing.T) {
	ctx := context.Background()
	alice, bob := uuid.NewString(), uuid.NewString()
	chat := createPrivateChat(t, now(), alice, bob)
	msg := createMessage(t, chat.ID, alice, "spam", now())

	if reported, _ := testRepo.HasReported(ctx, msg.ID, bob); reported {
		t.Fatal("HasReported() before report = true, want false")
	}

	report := &domain.MessageReport{
		ID: uuid.NewString(), MessageID: msg.ID, ChatID: chat.ID, ReporterID: bob,
		Category: domain.ReportCategorySpam, Comment: "ads", CreatedAt: now(),
	}
	if err := testRepo.CreateMessageReport(ctx, report); err != nil {
		t.Fatalf("CreateMessageReport() error: %v", err)
	}
	if reported, _ := testRepo.HasReported(ctx, msg.ID, bob); !reported {
		t.Error("HasReported() after report = false, want true")
	}
	if reported, _ := testRepo.HasReported(ctx, msg.ID, alice); reported {
		t.Error("HasReported(alice) = true, want false: only bob reported")
	}

	dup := *report
	dup.ID = uuid.NewString()
	if err := testRepo.CreateMessageReport(ctx, &dup); err != nil {
		t.Errorf("duplicate CreateMessageReport() error = %v, want nil", err)
	}
}
