package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yaoapp/gou/process"
	"github.com/yaoapp/gou/text"
	"github.com/yaoapp/kun/log"
	robottypes "github.com/yaoapp/yao/agent/robot/types"
	"github.com/yaoapp/yao/attachment"
	"github.com/yaoapp/yao/event"
	eventtypes "github.com/yaoapp/yao/event/types"
	"github.com/yaoapp/yao/messenger"
	messengerTypes "github.com/yaoapp/yao/messenger/types"
)

func init() {
	event.Register("robot", &robotHandler{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	})
}

// robotHandler processes all robot.* events.
type robotHandler struct {
	httpClient *http.Client
}

// Handle dispatches robot events by type.
func (h *robotHandler) Handle(ctx context.Context, ev *eventtypes.Event, resp chan<- eventtypes.Result) {
	switch ev.Type {
	case Delivery:
		h.handleDelivery(ctx, ev, resp)
	default:
		log.Debug("robot handler: unhandled event type=%s id=%s", ev.Type, ev.ID)
	}
}

// Shutdown gracefully shuts down the robot handler.
func (h *robotHandler) Shutdown(ctx context.Context) error {
	return nil
}

// handleDelivery routes delivery content to configured channels (email, webhook, process).
func (h *robotHandler) handleDelivery(ctx context.Context, ev *eventtypes.Event, resp chan<- eventtypes.Result) {
	var payload DeliveryPayload
	if err := ev.Should(&payload); err != nil {
		log.Error("delivery handler: invalid payload: %v", err)
		if ev.IsCall {
			resp <- eventtypes.Result{Err: err}
		}
		return
	}

	log.Info("delivery handler: execution=%s member=%s", payload.ExecutionID, payload.MemberID)

	content := payload.Content
	prefs := payload.Preferences
	if content == nil {
		log.Warn("delivery handler: nil content for execution=%s", payload.ExecutionID)
		if ev.IsCall {
			resp <- eventtypes.Result{Data: "no content"}
		}
		return
	}
	if prefs == nil {
		if ev.IsCall {
			resp <- eventtypes.Result{Data: "no preferences, skipped"}
		}
		return
	}

	deliveryCtx := &robottypes.DeliveryContext{
		MemberID:    payload.MemberID,
		ExecutionID: payload.ExecutionID,
		TeamID:      payload.TeamID,
	}

	var results []robottypes.ChannelResult
	var lastErr error

	if prefs.Email != nil && prefs.Email.Enabled {
		for _, target := range prefs.Email.Targets {
			r := h.sendEmail(ctx, content, target, deliveryCtx)
			results = append(results, r)
			if !r.Success && lastErr == nil {
				lastErr = fmt.Errorf("email delivery failed: %s", r.Error)
			}
		}
	}

	if prefs.Webhook != nil && prefs.Webhook.Enabled {
		for _, target := range prefs.Webhook.Targets {
			r := h.postWebhook(ctx, content, target, deliveryCtx)
			results = append(results, r)
			if !r.Success && lastErr == nil {
				lastErr = fmt.Errorf("webhook delivery failed: %s", r.Error)
			}
		}
	}

	if prefs.Process != nil && prefs.Process.Enabled {
		for _, target := range prefs.Process.Targets {
			r := h.callProcess(ctx, content, target, deliveryCtx)
			results = append(results, r)
			if !r.Success && lastErr == nil {
				lastErr = fmt.Errorf("process delivery failed: %s", r.Error)
			}
		}
	}

	if lastErr != nil {
		log.Error("delivery handler: partial failure execution=%s: %v", payload.ExecutionID, lastErr)
	}

	if ev.IsCall {
		resp <- eventtypes.Result{
			Data: map[string]interface{}{
				"execution_id": payload.ExecutionID,
				"results":      results,
			},
			Err: lastErr,
		}
	}
}

// ============================================================================
// Email
// ============================================================================

func (h *robotHandler) sendEmail(
	ctx context.Context,
	content *robottypes.DeliveryContent,
	target robottypes.EmailTarget,
	deliveryCtx *robottypes.DeliveryContext,
) robottypes.ChannelResult {
	now := time.Now()
	targetID := strings.Join(target.To, ",")
	if targetID == "" {
		targetID = "no-recipients"
	}

	result := robottypes.ChannelResult{
		Type:   robottypes.DeliveryEmail,
		Target: targetID,
		SentAt: &now,
	}

	svc := messenger.Instance
	if svc == nil {
		result.Error = "messenger service not available"
		return result
	}

	htmlBody, plainBody := buildEmailBody(target.Template, content)
	msg := &messengerTypes.Message{
		To:      target.To,
		Subject: buildEmailSubject(target.Subject, target.Template, content, deliveryCtx),
		Body:    plainBody,
		HTML:    htmlBody,
		Type:    messengerTypes.MessageTypeEmail,
	}

	attachments := convertAttachments(ctx, content.Attachments)
	if len(attachments) > 0 {
		msg.Attachments = attachments
	}

	channel := robottypes.DefaultEmailChannel()
	if err := svc.Send(ctx, channel, msg); err != nil {
		result.Error = err.Error()
		return result
	}

	result.Success = true
	result.Recipients = target.To
	return result
}

// ============================================================================
// Webhook
// ============================================================================

func (h *robotHandler) postWebhook(
	ctx context.Context,
	content *robottypes.DeliveryContent,
	target robottypes.WebhookTarget,
	deliveryCtx *robottypes.DeliveryContext,
) robottypes.ChannelResult {
	now := time.Now()
	result := robottypes.ChannelResult{
		Type:   robottypes.DeliveryWebhook,
		Target: target.URL,
		SentAt: &now,
	}

	payload := map[string]interface{}{
		"event":        "robot.delivery",
		"timestamp":    now.Format(time.RFC3339),
		"execution_id": deliveryCtx.ExecutionID,
		"member_id":    deliveryCtx.MemberID,
		"team_id":      deliveryCtx.TeamID,
		"trigger_type": deliveryCtx.TriggerType,
		"content": map[string]interface{}{
			"summary": content.Summary,
			"body":    content.Body,
		},
	}

	if len(content.Attachments) > 0 {
		info := make([]map[string]interface{}, 0, len(content.Attachments))
		for _, att := range content.Attachments {
			info = append(info, map[string]interface{}{
				"title":       att.Title,
				"description": att.Description,
				"task_id":     att.TaskID,
				"file":        att.File,
			})
		}
		payload["attachments"] = info
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal payload: %v", err)
		return result
	}

	method := target.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequestWithContext(ctx, method, target.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		return result
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range target.Headers {
		req.Header.Set(key, value)
	}

	if target.Secret != "" {
		signature := ComputeHMACSignature(payloadBytes, target.Secret)
		req.Header.Set("X-Yao-Signature", signature)
		req.Header.Set("X-Yao-Signature-Algorithm", "HMAC-SHA256")
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("webhook returned status %d: %s", resp.StatusCode, string(body))
		return result
	}

	result.Success = true
	result.Details = map[string]interface{}{
		"status_code": resp.StatusCode,
		"response":    string(body),
	}
	return result
}

// ============================================================================
// Process
// ============================================================================

func (h *robotHandler) callProcess(
	ctx context.Context,
	content *robottypes.DeliveryContent,
	target robottypes.ProcessTarget,
	deliveryCtx *robottypes.DeliveryContext,
) robottypes.ChannelResult {
	now := time.Now()
	result := robottypes.ChannelResult{
		Type:   robottypes.DeliveryProcess,
		Target: target.Process,
		SentAt: &now,
	}

	args := make([]interface{}, 0, 1+len(target.Args))
	args = append(args, map[string]interface{}{
		"content": map[string]interface{}{
			"summary":     content.Summary,
			"body":        content.Body,
			"attachments": content.Attachments,
		},
		"context": map[string]interface{}{
			"execution_id": deliveryCtx.ExecutionID,
			"member_id":    deliveryCtx.MemberID,
			"team_id":      deliveryCtx.TeamID,
			"trigger_type": deliveryCtx.TriggerType,
		},
	})
	args = append(args, target.Args...)

	proc, err := process.Of(target.Process, args...)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create process: %v", err)
		return result
	}
	proc.Context = ctx

	if err = proc.Execute(); err != nil {
		result.Error = err.Error()
		return result
	}

	result.Success = true
	result.Details = toJSONSerializable(proc.Value)
	return result
}

// ============================================================================
// Helpers
// ============================================================================

func toJSONSerializable(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	if _, err := json.Marshal(v); err != nil {
		return fmt.Sprintf("%v", v)
	}
	return v
}

func buildEmailSubject(subject, template string, content *robottypes.DeliveryContent, ctx *robottypes.DeliveryContext) string {
	if subject != "" {
		return subject
	}
	if content.Summary != "" {
		return content.Summary
	}
	return fmt.Sprintf("Execution %s Complete", ctx.ExecutionID)
}

func buildEmailBody(template string, content *robottypes.DeliveryContent) (string, string) {
	markdown := content.Body
	if markdown == "" {
		markdown = content.Summary
	}
	html, err := text.MarkdownToHTML(markdown)
	if err != nil {
		return markdown, markdown
	}
	return html, markdown
}

func convertAttachments(ctx context.Context, attachments []robottypes.DeliveryAttachment) []messengerTypes.Attachment {
	if len(attachments) == 0 {
		return nil
	}

	result := make([]messengerTypes.Attachment, 0, len(attachments))
	for _, att := range attachments {
		uploader, fileID, isWrapper := attachment.Parse(att.File)
		if !isWrapper {
			log.Warn("convertAttachments: skipping non-wrapper file value=%q title=%q", att.File, att.Title)
			continue
		}
		manager, ok := attachment.Managers[uploader]
		if !ok {
			log.Warn("convertAttachments: manager not found uploader=%q file=%q title=%q (available: %v)",
				uploader, att.File, att.Title, attachmentManagerKeys())
			continue
		}
		info, err := manager.Info(ctx, fileID)
		if err != nil {
			log.Warn("convertAttachments: failed to get file info fileID=%q uploader=%q: %v", fileID, uploader, err)
			continue
		}
		content, err := manager.Read(ctx, fileID)
		if err != nil {
			log.Warn("convertAttachments: failed to read file fileID=%q uploader=%q: %v", fileID, uploader, err)
			continue
		}

		// Prefer the semantic title from the delivery agent over the raw storage filename.
		// The storage filename may be an auto-generated zip name (e.g. output_xxx.zip),
		// while att.Title is the human-readable name set by the delivery agent.
		filename := info.Filename
		if att.Title != "" {
			// Keep the original file extension from storage so the email client
			// knows how to open it, but use the human-readable title as the base name.
			ext := ""
			if idx := strings.LastIndex(info.Filename, "."); idx >= 0 {
				ext = info.Filename[idx:]
			}
			titleExt := ""
			if idx := strings.LastIndex(att.Title, "."); idx >= 0 {
				titleExt = att.Title[idx:]
			}
			if titleExt != "" {
				// Title already has an extension — use it as-is.
				filename = att.Title
			} else {
				// Title has no extension — append the storage extension.
				filename = att.Title + ext
			}
		}

		log.Info("convertAttachments: added attachment filename=%q contentType=%q size=%d", filename, info.ContentType, len(content))
		result = append(result, messengerTypes.Attachment{
			Filename:    filename,
			ContentType: info.ContentType,
			Content:     content,
		})
	}
	return result
}

// attachmentManagerKeys returns registered attachment manager names for debug logging.
func attachmentManagerKeys() []string {
	keys := make([]string, 0, len(attachment.Managers))
	for k := range attachment.Managers {
		keys = append(keys, k)
	}
	return keys
}

// ComputeHMACSignature computes HMAC-SHA256 signature for webhook payload.
func ComputeHMACSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHMACSignature verifies the HMAC-SHA256 signature of a webhook payload.
func VerifyHMACSignature(payload []byte, secret, signature string) bool {
	expected := ComputeHMACSignature(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}
