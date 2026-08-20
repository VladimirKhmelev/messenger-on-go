package domain

import "errors"

var (
	ErrChatNotFound       = errors.New("chat not found")
	ErrNotChatMember      = errors.New("user is not a member of this chat")
	ErrEmptyMessage       = errors.New("message body must not be empty")
	ErrChatAlreadyExists  = errors.New("private chat between these users already exists")
	ErrCannotChatWithSelf = errors.New("cannot create a chat with yourself")
	ErrTargetUserNotFound = errors.New("target user not found")
	ErrMessageNotFound    = errors.New("message not found")
	ErrMessageDeleted     = errors.New("message has been deleted")
	ErrNotMessageSender   = errors.New("only the sender can edit or delete this message for everyone")
	ErrMessageNotInChat   = errors.New("message does not belong to this chat")
	ErrMissingChatKey     = errors.New("encrypted chat key must be provided for every participant")
)
