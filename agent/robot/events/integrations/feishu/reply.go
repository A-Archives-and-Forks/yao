package feishu

import (
	"context"
	"fmt"
	"strings"

	agentcontext "github.com/yaoapp/yao/agent/context"
	events "github.com/yaoapp/yao/agent/robot/events"
	"github.com/yaoapp/yao/attachment"
)

// Reply sends the assistant message back to the originating Feishu chat.
func (a *Adapter) Reply(ctx context.Context, msg *agentcontext.Message, metadata *events.MessageMetadata) error {
	if msg == nil || metadata == nil {
		return fmt.Errorf("nil message or metadata")
	}

	entry := a.resolveByChat(metadata)
	if entry == nil {
		return fmt.Errorf("no bot registered for feishu metadata (appID=%s)", metadata.AppID)
	}

	var replyToMsgID string
	if metadata.Extra != nil {
		if v, ok := metadata.Extra["feishu_message_id"]; ok {
			if s, ok := v.(string); ok {
				replyToMsgID = s
			}
		}
	}

	return a.sendContent(ctx, entry, metadata.ChatID, replyToMsgID, msg.Content)
}

func (a *Adapter) sendContent(ctx context.Context, entry *botEntry, chatID, replyToMsgID string, content interface{}) error {
	switch c := content.(type) {
	case string:
		if strings.TrimSpace(c) == "" {
			return nil
		}
		if replyToMsgID != "" {
			_, err := entry.bot.ReplyTextMessage(ctx, replyToMsgID, c)
			return err
		}
		_, err := entry.bot.SendTextMessage(ctx, chatID, c)
		return err

	case []interface{}:
		return a.sendParts(ctx, entry, chatID, replyToMsgID, c)

	default:
		parts, ok := toContentParts(content)
		if ok {
			return a.sendPartsTyped(ctx, entry, chatID, replyToMsgID, parts)
		}
		text := fmt.Sprintf("%v", content)
		_, err := entry.bot.SendTextMessage(ctx, chatID, text)
		return err
	}
}

func (a *Adapter) sendParts(ctx context.Context, entry *botEntry, chatID, replyToMsgID string, parts []interface{}) error {
	var textBuf strings.Builder
	for _, part := range parts {
		m, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		partType, _ := m["type"].(string)
		switch partType {
		case "text":
			if text, ok := m["text"].(string); ok {
				textBuf.WriteString(text)
			}
		case "image_url":
			if err := a.flushText(ctx, entry, chatID, replyToMsgID, &textBuf); err != nil {
				return err
			}
		case "file":
			if err := a.flushText(ctx, entry, chatID, replyToMsgID, &textBuf); err != nil {
				return err
			}
			if fileURL, ok := m["file_url"].(string); ok && fileURL != "" {
				if err := a.sendFileContent(ctx, entry, chatID, fileURL); err != nil {
					log.Error("feishu reply: send file: %v", err)
				}
			}
		}
	}
	return a.flushText(ctx, entry, chatID, replyToMsgID, &textBuf)
}

func (a *Adapter) sendPartsTyped(ctx context.Context, entry *botEntry, chatID, replyToMsgID string, parts []agentcontext.ContentPart) error {
	var textBuf strings.Builder
	for _, part := range parts {
		switch part.Type {
		case agentcontext.ContentText:
			textBuf.WriteString(part.Text)
		case agentcontext.ContentImageURL, agentcontext.ContentFile:
			if err := a.flushText(ctx, entry, chatID, replyToMsgID, &textBuf); err != nil {
				return err
			}
		}
	}
	return a.flushText(ctx, entry, chatID, replyToMsgID, &textBuf)
}

func (a *Adapter) flushText(ctx context.Context, entry *botEntry, chatID, replyToMsgID string, buf *strings.Builder) error {
	if buf.Len() == 0 {
		return nil
	}
	text := buf.String()
	buf.Reset()

	if replyToMsgID != "" {
		_, err := entry.bot.ReplyTextMessage(ctx, replyToMsgID, text)
		return err
	}
	_, err := entry.bot.SendTextMessage(ctx, chatID, text)
	return err
}

func (a *Adapter) sendFileContent(ctx context.Context, entry *botEntry, chatID, fileURL string) error {
	if strings.Contains(fileURL, "://") && !strings.HasPrefix(fileURL, "http") {
		_, fileID, ok := attachment.Parse(fileURL)
		if !ok {
			return fmt.Errorf("parse wrapper: invalid format %s", fileURL)
		}
		_, _ = fileID, chatID
		log.Warn("feishu: file wrapper send not yet implemented, wrapper=%s", fileURL)
	}
	return nil
}

func toContentParts(content interface{}) ([]agentcontext.ContentPart, bool) {
	parts, ok := content.([]agentcontext.ContentPart)
	return parts, ok
}

func (a *Adapter) resolveByChat(metadata *events.MessageMetadata) *botEntry {
	if metadata.AppID != "" {
		if entry, ok := a.resolveByAppID(metadata.AppID); ok {
			return entry
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, entry := range a.bots {
		return entry
	}
	return nil
}
