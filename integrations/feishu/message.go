package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// SendTextMessage sends a text message to a chat.
func (b *Bot) SendTextMessage(ctx context.Context, chatID, text string) (string, error) {
	content, _ := json.Marshal(map[string]string{"text": text})
	return b.sendMessage(ctx, "chat_id", chatID, "text", string(content))
}

// SendTextToUser sends a text message to a user by open_id.
func (b *Bot) SendTextToUser(ctx context.Context, openID, text string) (string, error) {
	content, _ := json.Marshal(map[string]string{"text": text})
	return b.sendMessage(ctx, "open_id", openID, "text", string(content))
}

// SendImageMessage sends an image by image_key to a chat.
func (b *Bot) SendImageMessage(ctx context.Context, chatID, imageKey string) (string, error) {
	content, _ := json.Marshal(map[string]string{"image_key": imageKey})
	return b.sendMessage(ctx, "chat_id", chatID, "image", string(content))
}

// SendFileMessage sends a file by file_key to a chat.
func (b *Bot) SendFileMessage(ctx context.Context, chatID, fileKey string) (string, error) {
	content, _ := json.Marshal(map[string]string{"file_key": fileKey})
	return b.sendMessage(ctx, "chat_id", chatID, "file", string(content))
}

// ReplyTextMessage replies to a message with text.
func (b *Bot) ReplyTextMessage(ctx context.Context, messageID, text string) (string, error) {
	content, _ := json.Marshal(map[string]string{"text": text})
	return b.replyMessage(ctx, messageID, "text", string(content))
}

func (b *Bot) sendMessage(ctx context.Context, receiveIDType, receiveID, msgType, content string) (string, error) {
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	resp, err := b.client.Im.Message.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("feishu send message: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("feishu send message: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data != nil && resp.Data.MessageId != nil {
		return *resp.Data.MessageId, nil
	}
	return "", nil
}

func (b *Bot) replyMessage(ctx context.Context, messageID, msgType, content string) (string, error) {
	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	resp, err := b.client.Im.Message.Reply(ctx, req)
	if err != nil {
		return "", fmt.Errorf("feishu reply message: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("feishu reply message: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data != nil && resp.Data.MessageId != nil {
		return *resp.Data.MessageId, nil
	}
	return "", nil
}
