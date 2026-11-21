package api

import (
	"net/http"
	"testing"
)

func TestSubscribeAndManageKeepTheDailyOrWeeklyChoice(t *testing.T) {
	srv, n := subscribeFixture(t, true)
	if got := post(t, srv, "/subscribe", `{"email":"reader@example.com","frequency":"weekly"}`); got != http.StatusAccepted {
		t.Fatalf("subscribe weekly = %d", got)
	}
	token := n.tokens[0]

	code, body := getJSONBody(t, mustReq("GET", srv.URL+"/subscription?token="+token, ""))
	if code != 200 || body["frequency"] != "weekly" {
		t.Fatalf("GET /subscription = %d %v; want frequency weekly", code, body)
	}

	// Saving without a frequency keeps it; naming one changes it; a bad one is refused.
	code, body = getJSONBody(t, mustReq("POST", srv.URL+"/subscription", `{"token":"`+token+`","mode":"1min"}`))
	if code != 200 || body["frequency"] != "weekly" {
		t.Fatalf("save without a frequency = %d %v; want it kept as weekly", code, body)
	}
	code, body = getJSONBody(t, mustReq("POST", srv.URL+"/subscription", `{"token":"`+token+`","mode":"1min","frequency":"daily"}`))
	if code != 200 || body["frequency"] != "daily" {
		t.Fatalf("save daily = %d %v", code, body)
	}
	if code, _ = getJSONBody(t, mustReq("POST", srv.URL+"/subscription", `{"token":"`+token+`","mode":"1min","frequency":"monthly"}`)); code != http.StatusBadRequest {
		t.Fatalf("an unknown frequency = %d, want 400", code)
	}
}
