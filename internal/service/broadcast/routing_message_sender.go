package broadcast

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// routingMessageSender sends through the existing queue/direct sender for every
// provider except the beta Resend Broadcasts provider, which has its own batch-only
// transport.
type routingMessageSender struct {
	normal     MessageSender
	broadcasts MessageSender
}

func NewRoutingMessageSender(normal, broadcasts MessageSender) MessageSender {
	return &routingMessageSender{normal: normal, broadcasts: broadcasts}
}

func (r *routingMessageSender) SendToRecipient(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	workspaceSecretKey string,
	endpoint string,
	decision domain.EmailTrackingDecision,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcast *domain.Broadcast,
	messageID string,
	email string,
	template *domain.Template,
	data map[string]interface{},
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
	contactLanguage string,
	workspaceDefaultLanguage string,
) error {
	if emailProvider != nil && emailProvider.Kind == domain.EmailProviderKindResendBroadcasts {
		return fmt.Errorf("resend broadcasts provider is batch-only")
	}
	return r.normal.SendToRecipient(ctx, workspaceID, integrationID, workspaceSecretKey, endpoint, decision, webAnalytics, broadcast, messageID, email, template, data, emailProvider, timeoutAt, contactLanguage, workspaceDefaultLanguage)
}

func (r *routingMessageSender) SendBatch(
	ctx context.Context,
	workspaceID string,
	integrationID string,
	workspaceSecretKey string,
	endpoint string,
	websiteURL string,
	trackingPolicy domain.EmailTrackingPolicy,
	webAnalytics *domain.WebAnalyticsSettings,
	broadcastID string,
	recipients []*domain.ContactWithList,
	templates map[string]*domain.Template,
	emailProvider *domain.EmailProvider,
	timeoutAt time.Time,
	workspaceDefaultLanguage string,
) (int, int, error) {
	if emailProvider != nil && emailProvider.Kind == domain.EmailProviderKindResendBroadcasts {
		return r.broadcasts.SendBatch(ctx, workspaceID, integrationID, workspaceSecretKey, endpoint, websiteURL, trackingPolicy, webAnalytics, broadcastID, recipients, templates, emailProvider, timeoutAt, workspaceDefaultLanguage)
	}
	return r.normal.SendBatch(ctx, workspaceID, integrationID, workspaceSecretKey, endpoint, websiteURL, trackingPolicy, webAnalytics, broadcastID, recipients, templates, emailProvider, timeoutAt, workspaceDefaultLanguage)
}
