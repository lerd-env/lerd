package ui

import "testing"

func TestNginxUpdateTransition(t *testing.T) {
	nginxUpdatePrev = nil
	if nginxUpdateTransition([]byte(`{"nginx":{"running":true,"update_available":true}}`)) {
		t.Error("the first snapshot has nothing to compare against and must stay quiet")
	}
	nginxUpdatePrev = boolPtr(false)
	if !nginxUpdateTransition([]byte(`{"nginx":{"running":true,"update_available":true}}`)) {
		t.Error("a newly flagged update must notify")
	}
	if nginxUpdateTransition([]byte(`{"nginx":{"running":true,"update_available":true}}`)) {
		t.Error("an update already announced must not notify again")
	}
	if nginxUpdateTransition([]byte(`{"nginx":{"running":true}}`)) {
		t.Error("a cleared flag must not notify")
	}
}

func TestNotificationForNginxUpdate_Shape(t *testing.T) {
	n := notificationForNginxUpdate()
	if n.Kind != "update_available" || n.URL != "#system/nginx" || n.BodyKey != "notify_nginx_update_body" {
		t.Errorf("got kind=%q url=%q bodyKey=%q", n.Kind, n.URL, n.BodyKey)
	}
}

func boolPtr(b bool) *bool { return &b }
