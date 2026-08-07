package adminweb

import (
	"bytes"
	"encoding/gob"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardfund/service/donations"
	"boardfund/service/members"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// imageRig drives the real route through the real session middleware.
//
// donationService is nil on purpose. Every case below is refused before the
// handler reaches it, which is the point being made: these are decisions the
// handler takes on its own, and a nil here means a test that accidentally got
// past them fails loudly rather than quietly exercising something else.
func imageRig(t *testing.T) func(fundID string, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	// scs gob-encodes what it stores. Without this it cannot commit the session and
	// answers 500 over a handler that worked -- which is exactly how a set of
	// assertions that only read the body came to pass against the wrong path.
	gob.Register(members.Member{})

	sessions := scs.New()
	handlers := &AdminHandlers{sessionManager: sessions}

	router := http.NewServeMux()
	router.HandleFunc("POST /admin/fund/image/{id}", func(w http.ResponseWriter, r *http.Request) {
		sessions.Put(r.Context(), "member", members.Member{ID: uuid.New()})

		handlers.setFundImage(w, r)
	})

	return func(fundID, contentType string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/admin/fund/image/"+fundID, bytes.NewReader(body))
		request.Header.Set("Content-Type", contentType)

		recorder := httptest.NewRecorder()
		sessions.LoadAndSave(router).ServeHTTP(recorder, request)

		return recorder
	}
}

// multipartBody builds a well-formed upload of the given size.
func multipartBody(t *testing.T, size int) (string, []byte) {
	t.Helper()

	var out bytes.Buffer
	writer := multipart.NewWriter(&out)

	part, err := writer.CreateFormFile("image", "picture.jpg")
	require.NoError(t, err)

	_, err = part.Write(bytes.Repeat([]byte{0x41}, size))
	require.NoError(t, err)

	require.NoError(t, writer.Close())

	return writer.FormDataContentType(), out.Bytes()
}

// Every failure parsing the upload used to be reported as "too large", with a
// 400. One of those is a size problem and the rest are not, and telling somebody
// their file is too big sends them off to shrink a picture that was never the
// trouble.
func TestUploadFailuresSayWhatWentWrong(t *testing.T) {
	post := imageRig(t)
	fundID := uuid.NewString()

	t.Run("an upload past the limit is too large", func(t *testing.T) {
		contentType, body := multipartBody(t, donations.MaxImageBytes+4096)

		recorder := post(fundID, contentType, body)

		// 413, not 400. The request was well formed and there was simply too much of
		// it, which is a different thing from one the server could not read.
		require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		require.Contains(t, recorder.Body.String(), "megabytes")
	})

	t.Run("a body that is not multipart is not a size problem", func(t *testing.T) {
		recorder := post(fundID, "multipart/form-data; boundary=nothing-like-this",
			[]byte("this is not a multipart body at all"))

		require.Equal(t, http.StatusBadRequest, recorder.Code)

		html := recorder.Body.String()
		require.Contains(t, html, "could not read that upload")
		require.NotContains(t, html, "megabytes",
			"a malformed body was blamed on its size")
	})

	t.Run("a missing boundary is not a size problem", func(t *testing.T) {
		recorder := post(fundID, "multipart/form-data", []byte("anything"))

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.NotContains(t, recorder.Body.String(), "megabytes")
	})

	t.Run("no file chosen says so", func(t *testing.T) {
		var out bytes.Buffer
		writer := multipart.NewWriter(&out)
		require.NoError(t, writer.WriteField("something", "else"))
		require.NoError(t, writer.Close())

		recorder := post(fundID, writer.FormDataContentType(), out.Bytes())

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Contains(t, recorder.Body.String(), "choose an image")
	})

	t.Run("a fund id that is not one", func(t *testing.T) {
		contentType, body := multipartBody(t, 32)

		recorder := post("not-a-uuid", contentType, body)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Contains(t, recorder.Body.String(), "not a fund")
	})

	// Whatever the refusal, it is swapped into the control by htmx. A whole layout
	// document put inside it is not something a browser can make sense of.
	t.Run("every refusal is a fragment", func(t *testing.T) {
		contentType, body := multipartBody(t, donations.MaxImageBytes+4096)

		for name, recorder := range map[string]*httptest.ResponseRecorder{
			"too large":   post(fundID, contentType, body),
			"unreadable":  post(fundID, "multipart/form-data; boundary=x", []byte("junk")),
			"no file":     post(fundID, contentType, []byte{}),
			"bad fund id": post("not-a-uuid", contentType, body),
		} {
			html := recorder.Body.String()

			for _, marker := range []string{"<html", "<head", "<body", "<nav"} {
				if strings.Contains(html, marker) {
					t.Errorf("%s answered with a document (%s)", name, marker)
				}
			}

			if !strings.Contains(html, `id="fund-image-control"`) {
				t.Errorf("%s should redraw the control so it can be tried again", name)
			}
		}
	})
}
