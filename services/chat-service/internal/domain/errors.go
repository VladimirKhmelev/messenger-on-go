package domain

import "errors"

var (
	ErrChatNotFound               = errors.New("chat not found")
	ErrNotChatMember              = errors.New("user is not a member of this chat")
	ErrEmptyMessage               = errors.New("message body must not be empty")
	ErrChatAlreadyExists          = errors.New("private chat between these users already exists")
	ErrCannotChatWithSelf         = errors.New("cannot create a chat with yourself")
	ErrTargetUserNotFound         = errors.New("target user not found")
	ErrMessageNotFound            = errors.New("message not found")
	ErrMessageDeleted             = errors.New("message has been deleted")
	ErrNotMessageSender           = errors.New("only the sender can edit or delete this message for everyone")
	ErrMessageNotInChat           = errors.New("message does not belong to this chat")
	ErrMissingChatKey             = errors.New("encrypted chat key must be provided for every participant")
	ErrMessageTooLarge            = errors.New("message body exceeds maximum allowed size")
	ErrTooManyMessages            = errors.New("too many messages sent, slow down")
	ErrTooFewMembers              = errors.New("a group chat needs at least two other members")
	ErrTooManyMembers             = errors.New("group chat member limit exceeded")
	ErrGroupNameRequired          = errors.New("group chat name must not be empty")
	ErrNotChatAdmin               = errors.New("only a chat admin can perform this action")
	ErrAlreadyMember              = errors.New("user is already a member of this chat")
	ErrNotGroupChat               = errors.New("this operation is only valid for group chats")
	ErrCannotRemoveCreator        = errors.New("the group creator cannot be removed")
	ErrOnlyCreatorCanManageAdmins = errors.New("only the group creator can promote, demote, or remove an admin")
	ErrInvalidRole                = errors.New("role must be admin or member")
)
