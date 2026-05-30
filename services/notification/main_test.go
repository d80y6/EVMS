package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultNotificationConfig()
	if config.NATSURL != "nats://nats:4222" {
		t.Errorf("default NATSURL = %q, want %q", config.NATSURL, "nats://nats:4222")
	}
}

func TestNotificationTypes(t *testing.T) {
	if NotificationTypeEmail != "email" {
		t.Errorf("NotificationTypeEmail = %q, want %q", NotificationTypeEmail, "email")
	}
	if NotificationTypeWebhook != "webhook" {
		t.Errorf("NotificationTypeWebhook = %q, want %q", NotificationTypeWebhook, "webhook")
	}
	if NotificationTypePush != "push" {
		t.Errorf("NotificationTypePush = %q, want %q", NotificationTypePush, "push")
	}
}
