package dingtalk

import (
	"context"
	"fmt"
	"strings"

	agentcontext "github.com/yaoapp/yao/agent/context"
	events "github.com/yaoapp/yao/agent/robot/events"
	dtapi "github.com/yaoapp/yao/integrations/dingtalk"
)

// Reply sends the assistant message back to the originating DingTalk conversation.
func (a *Adapter) Reply(ctx context.Context, msg *agentcontext.Message, metadata *events.MessageMetadata) error {
	if msg == nil || metadata == nil {
		return fmt.Errorf("nil message or metadata")
	}

	var sessionWebhook string
	if metadata.Extra != nil {
		if v, ok := metadata.Extra["session_webhook"]; ok {
			if s, ok := v.(string); ok {
				sessionWebhook = s
			}
		}
	}

	if sessionWebhook == "" {
		return fmt.Errorf("no session_webhook in metadata for dingtalk reply")
	}

	return sendContent(ctx, sessionWebhook, msg.Content)
}

func sendContent(ctx context.Context, sessionWebhook string, content interface{}) error {
	switch c := content.(type) {
	case string:
		if strings.TrimSpace(c) == "" {
			return nil
		}
		return dtapi.SendMarkdownMessage(ctx, sessionWebhook, "Reply", c)

	case []interface{}:
		return sendParts(ctx, sessionWebhook, c)

	default:
		parts, ok := toContentParts(content)
		if ok {
			return sendPartsTyped(ctx, sessionWebhook, parts)
		}
		return dtapi.SendTextMessage(ctx, sessionWebhook, fmt.Sprintf("%v", content))
	}
}

func sendParts(ctx context.Context, sessionWebhook string, parts []interface{}) error {
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
			if err := flushText(ctx, sessionWebhook, &textBuf); err != nil {
				return err
			}
		case "file":
			if err := flushText(ctx, sessionWebhook, &textBuf); err != nil {
				return err
			}
		}
	}
	return flushText(ctx, sessionWebhook, &textBuf)
}

func sendPartsTyped(ctx context.Context, sessionWebhook string, parts []agentcontext.ContentPart) error {
	var textBuf strings.Builder
	for _, part := range parts {
		switch part.Type {
		case agentcontext.ContentText:
			textBuf.WriteString(part.Text)
		case agentcontext.ContentImageURL, agentcontext.ContentFile:
			if err := flushText(ctx, sessionWebhook, &textBuf); err != nil {
				return err
			}
		}
	}
	return flushText(ctx, sessionWebhook, &textBuf)
}

func flushText(ctx context.Context, sessionWebhook string, buf *strings.Builder) error {
	if buf.Len() == 0 {
		return nil
	}
	text := buf.String()
	buf.Reset()
	return dtapi.SendMarkdownMessage(ctx, sessionWebhook, "Reply", text)
}

func toContentParts(content interface{}) ([]agentcontext.ContentPart, bool) {
	parts, ok := content.([]agentcontext.ContentPart)
	return parts, ok
}
