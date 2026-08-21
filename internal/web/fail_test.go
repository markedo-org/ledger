package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/markedo-org/ledger/internal/app"
)

func failWith(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks", nil)
	(&Server{}).fail(c, err)
	return rec
}

// Handlers used to hand err.Error() to the client whatever it was, so a SQLite
// message naming a table, or a filesystem path, went out over the wire.
func TestAnUnclassifiedErrorTellsTheClientNothing(t *testing.T) {
	rec := failWith(t, errors.New("no such table: tasks_v2 in /opt/ledger/ledger.sqlite"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, leak := range []string{"tasks_v2", "/opt/ledger", "no such table"} {
		if strings.Contains(body, leak) {
			t.Fatalf("the response carried %q: %s", leak, body)
		}
	}
	if !strings.Contains(body, "internal error") || !strings.Contains(body, `"ref"`) {
		t.Fatalf("want a generic message and a reference to find in the log, got %s", body)
	}
}

// The errors we wrote ourselves are the useful ones, and they stay.
func TestOurOwnErrorsStillReachTheCaller(t *testing.T) {
	cases := []struct {
		err  error
		code int
		want string
	}{
		{app.ErrInvalid, http.StatusBadRequest, "invalid"},
		{app.ErrNotFound, http.StatusNotFound, "not found"},
		{app.ErrForbidden, http.StatusForbidden, "forbidden"},
		{app.ErrConflict, http.StatusConflict, "conflict"},
		{app.ErrUnauthorized, http.StatusUnauthorized, "unauthorized"},
		{app.ErrPolicy, http.StatusUnprocessableEntity, "policy"},
	}
	for _, c := range cases {
		rec := failWith(t, c.err)
		if rec.Code != c.code {
			t.Fatalf("%v: status %d, want %d", c.err, rec.Code, c.code)
		}
		if !strings.Contains(rec.Body.String(), c.want) {
			t.Fatalf("%v: body %s", c.err, rec.Body.String())
		}
	}
}

func TestABodyOverTheCapIsRefusedWithItsSize(t *testing.T) {
	rec := failWith(t, &http.MaxBytesError{Limit: MaxRequestBody})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "1048576") {
		t.Fatalf("the caller should be told the limit: %s", rec.Body.String())
	}
}
