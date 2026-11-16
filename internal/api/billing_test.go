package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

const testSecret = "whsec_test"

func signed(body string) string {
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(body))

	return hex.EncodeToString(mac.Sum(nil))
}

func paidBody(event, payment, email string) string { return paidFor(event, payment, email, 9900) }

func paidFor(event, payment, email string, paise int) string {
	return `{"event":"` + event + `","payload":{"payment":{"entity":{"id":"` + payment + `","email":"` + email + `","amount":` + strconv.Itoa(paise) + `,"currency":"INR","status":"captured"}},"payment_link":{"entity":{"customer":{"email":"link@x.test"}}}}}`
}

func TestRazorpayWebhookMakesAProMemberOnce(t *testing.T) {
	st := store.NewMemory()
	var mu sync.Mutex
	var notified []string
	srv := httptest.NewServer(NewHandler(Deps{
		Frontier: frontier.NewMemory(nil), Store: st, RazorpaySecret: testSecret, ProPlans: map[int]int{9900: 31, 24900: 92},
		ProNotify: func(_ context.Context, email, token string, until time.Time) {
			mu.Lock()
			defer mu.Unlock()
			notified = append(notified, email)
			if token == "" || until.Before(time.Now()) {
				t.Errorf("notify got token %q until %v", token, until)
			}
		},
	}))
	t.Cleanup(srv.Close)

	send := func(body, sig string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/billing/razorpay", bytes.NewBufferString(body))
		req.Header.Set("X-Razorpay-Signature", sig)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()

		return res.StatusCode
	}

	body := paidBody("payment_link.paid", "pay_1", "Buyer@X.test")
	if got := send(body, "deadbeef"); got != http.StatusBadRequest {
		t.Fatalf("bad signature = %d, want 400", got)
	}
	if got := send(body, ""); got != http.StatusBadRequest {
		t.Fatalf("missing signature = %d, want 400", got)
	}
	if active, _ := st.ActiveProTokens(context.Background()); len(active) != 0 {
		t.Fatalf("a refused webhook made a member: %v", active)
	}

	if got := send(body, signed(body)); got != http.StatusOK {
		t.Fatalf("signed webhook = %d, want 200", got)
	}
	// Razorpay also sends payment.captured for the same payment, and retries: neither extends again.
	dup := paidBody("payment.captured", "pay_1", "buyer@x.test")
	if got := send(dup, signed(dup)); got != http.StatusOK {
		t.Fatalf("duplicate = %d, want 200", got)
	}
	active, _ := st.ActiveProTokens(context.Background())
	if len(active) != 1 || active["buyer@x.test"] == "" {
		t.Fatalf("active = %v, want buyer@x.test (the address is lowercased)", active)
	}
	m, _, _ := st.ProByToken(context.Background(), active["buyer@x.test"])
	if time.Until(m.Until) > 32*24*time.Hour {
		t.Fatalf("one payment extended to %v, want about 31 days", m.Until)
	}
	if len(notified) != 1 {
		t.Fatalf("notified %v, want one welcome mail", notified)
	}

	// An event we do not use is acknowledged and does nothing.
	other := paidBody("refund.created", "pay_9", "other@x.test")
	if got := send(other, signed(other)); got != http.StatusOK {
		t.Fatalf("other event = %d, want 200", got)
	}
	// Without a usable email on the payment, the payment link's customer email is used.
	noEmail := paidBody("payment_link.paid", "pay_2", "")
	if got := send(noEmail, signed(noEmail)); got != http.StatusOK {
		t.Fatalf("link customer = %d", got)
	}
	if active, _ := st.ActiveProTokens(context.Background()); len(active) != 2 || active["link@x.test"] == "" {
		t.Fatalf("active = %v, want the payment link's customer too", active)
	}
}

func TestRazorpayAmountDecidesTheDaysBought(t *testing.T) {
	st := store.NewMemory()
	srv := httptest.NewServer(NewHandler(Deps{
		Frontier: frontier.NewMemory(nil), Store: st, RazorpaySecret: testSecret, ProPlans: map[int]int{9900: 31, 24900: 92},
	}))
	t.Cleanup(srv.Close)

	send := func(body string) {
		t.Helper()
		req, _ := http.NewRequest("POST", srv.URL+"/billing/razorpay", bytes.NewBufferString(body))
		req.Header.Set("X-Razorpay-Signature", signed(body))
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("webhook = %v, %v; want 200", res, err)
		}
		res.Body.Close()
	}
	until := func(email string) time.Duration {
		active, _ := st.ActiveProTokens(context.Background())
		m, ok, _ := st.ProByToken(context.Background(), active[email])
		if !ok {
			return 0
		}

		return time.Until(m.Until)
	}
	near := func(got time.Duration, days int) bool {
		return got > time.Duration(days-1)*24*time.Hour && got < time.Duration(days+1)*24*time.Hour
	}

	send(paidFor("payment_link.paid", "pay_m", "month@x.test", 9900))
	send(paidFor("payment_link.paid", "pay_q", "quarter@x.test", 24900))
	if got := until("month@x.test"); !near(got, 31) {
		t.Errorf("99 rupees bought %v, want about 31 days", got)
	}
	if got := until("quarter@x.test"); !near(got, 92) {
		t.Errorf("249 rupees bought %v, want about 92 days", got)
	}

	// A payment that is not captured INR buys nothing, even with a plan's number as the amount.
	send(strings.Replace(paidFor("payment_link.paid", "pay_usd", "usd@x.test", 9900), `"currency":"INR"`, `"currency":"USD"`, 1))
	send(strings.Replace(paidFor("payment_link.paid", "pay_auth", "auth@x.test", 9900), `"status":"captured"`, `"status":"authorized"`, 1))
	if got := until("usd@x.test") + until("auth@x.test"); got != 0 {
		t.Errorf("a non-INR or uncaptured payment made a member for %v", got)
	}

	// An amount that is in no plan is acknowledged and buys nothing.
	send(paidFor("payment_link.paid", "pay_odd", "odd@x.test", 12345))
	if got := until("odd@x.test"); got != 0 {
		t.Errorf("an unknown amount made a member for %v", got)
	}
}

func TestBillingRouteNeedsASecret(t *testing.T) {
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: store.NewMemory()}))
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/billing/razorpay", "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("webhook without a secret = %d, want it not served", res.StatusCode)
	}
}

func TestProStatus(t *testing.T) {
	f := newFixture(t)
	m, _, _ := f.store.ProExtend(context.Background(), "pay_1", "a@x.test", 5, "tok-a")

	status, body, hdr := f.do(t, "GET", "/pro?token=tok-a", "")
	if status != 200 || body["active"] != true || body["until"] != m.Until.UTC().Format("2006-01-02") || hdr.Get("Cache-Control") != "no-store" {
		t.Fatalf("pro = %d %v %v", status, body, hdr)
	}
	for path, want := range map[string]int{"/pro?token=nope": 404, "/pro": 400} {
		if status, _, _ := f.do(t, "GET", path, ""); status != want {
			t.Errorf("%s = %d, want %d", path, status, want)
		}
	}
}
