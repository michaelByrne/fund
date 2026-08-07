package homeweb

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/web/common"
	"boardfund/web/mux"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

const internalErrMessage = "internal error"

type FundHandlers struct {
	donationService *donations.DonationService
	sessionManager  *scs.SessionManager
	withAuth        func(http.HandlerFunc) http.HandlerFunc
	logger          *slog.Logger
	productID       string
	clientID        string
}

func NewFundHandlers(
	donationService *donations.DonationService,
	sessionManager *scs.SessionManager,
	withAuth func(http.HandlerFunc) http.HandlerFunc,
	logger *slog.Logger,
	productID, clientID string,
) *FundHandlers {
	return &FundHandlers{
		donationService: donationService,
		sessionManager:  sessionManager,
		withAuth:        withAuth,
		logger:          logger,
		productID:       productID,
		clientID:        clientID,
	}
}

func (h *FundHandlers) Register(r *mux.Router) {
	r.HandleFunc("/donation/plan", h.withAuth(h.createDonationPlan))
	r.HandleFunc("/donation/once", h.withAuth(h.createOneTimeDonation))
	r.HandleFunc("/donation/plan/complete", h.withAuth(h.completeRecurringDonation))
	r.HandleFunc("/donation/once/complete", h.withAuth(h.completeOneTimeDonation))
	r.HandleFunc("/donation/once/initiate", h.withAuth(h.initiateOneTimeDonation))
	r.HandleFunc("/donation/success", h.withAuth(h.donationSuccess))
	r.HandleFunc("GET /donations", h.withAuth(h.myDonations))
	// No auth: it is shown on pages that have it, and would be a broken box on any
	// that do not.
	r.HandleFunc("GET /fund/{fundId}/image/{hash}", h.fundImage)
	r.HandleFunc("POST /fund/{fundId}/note", h.withAuth(h.saveFundNote))
	r.HandleFunc("POST /fund/{fundId}/note/remove", h.withAuth(h.removeOwnFundNote))
	r.HandleFunc("POST /donation/cancel/{id}", h.withAuth(h.cancelMyDonation))
	r.HandleFunc("/donate/{fundId}", h.withAuth(h.donate))
	r.HandleFunc("/error", h.error)
	r.HandleFunc("/ping", h.ping)
	r.HandleFunc("/about", h.about)
	r.HandleFunc("/fund/{fundId}/summary", h.withAuth(h.closedFundSummary))
	r.HandleFunc("/", h.withAuth(h.home))
}

func (h *FundHandlers) error(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(nil, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	errorText := r.FormValue("error")

	common.ErrorMessage(nil, errorText, "/", r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) about(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		About(nil, r.URL.Path).Render(ctx, w)

		return
	}

	About(&member, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) initiateOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount_cents")
	amountCents, err := strconv.Atoi(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund_id")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	providerPaymentID, err := h.donationService.InitiateDonation(ctx, fundUUID, int32(amountCents))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	sendJSON(w, http.StatusOK, initDonationResponse{ProviderOrderID: providerPaymentID})
}

type initDonationResponse struct {
	ProviderOrderID string `json:"orderId"`
}

func (h *FundHandlers) createOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	Paypal(*fund, amountCents, h.clientID).Render(ctx, w)
}

func (h *FundHandlers) donate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		// nil, not &member. The assertion failed, so member is the zero value, and
		// a non-nil pointer to it reads as logged in: the nav renders logout and
		// hides login, leaving the one link this visitor needs off the page.
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundIDStr := r.PathValue("fundId")
	fundID, err := uuid.Parse(fundIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	notes, err := h.donationService.ListFundNotes(ctx, fund.ID)
	if err != nil {
		// Somebody came here to give, and the notes are not why. Logged, and the
		// section renders empty rather than costing them the donation form.
		h.logger.Error("failed to list fund notes", slog.String("error", err.Error()))
	}

	// Not fatal: somebody came here to give, and a missing picture is a smaller
	// wrong than a page that will not load.
	image, err := h.donationService.GetFundImage(ctx, fund.ID)
	if err != nil {
		image = nil
	}

	w.Header().Set("HX-Redirect", r.URL.Path)
	Fund(*fund, fund.Stats, notes, image, &member, r.URL.Path).Render(ctx, w)
}

// saveFundNote writes or replaces the signed-in donor's note on a fund.
//
// The member comes from the session and never from the request, so a caller can
// only ever write their own. The fund is not loaded here: eligibility already
// answers for a fund that is not there, since nobody has given to one.
func (h *FundHandlers) saveFundNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/login")

		return
	}

	fundID, err := uuid.Parse(r.PathValue("fundId"))
	if err != nil {
		h.badNote(ctx, w, http.StatusBadRequest, noteEditorID(fundID), fundID, nil, "that is not a fund")

		return
	}

	if err = r.ParseForm(); err != nil {
		h.badNote(ctx, w, http.StatusBadRequest, noteEditorID(fundID), fundID, nil, "we could not read that")

		return
	}

	body := r.FormValue("body")
	anonymous := r.FormValue("anonymous") == "true"
	editorID := noteEditor(r, fundID)

	note, err := h.donationService.SaveFundNote(ctx, fundID, member.ID, body, anonymous)
	if err != nil {
		// Each refusal says what it was. "Something went wrong" would leave a donor
		// retyping a note the server was never going to take.
		status, message := noteFailure(err)
		if message == "" {
			h.logger.Error("failed to save fund note", slog.String("error", err.Error()))

			message = "we could not save your note. please try again."
		}

		// Carrying what they typed, so a refusal does not throw their words away.
		h.badNote(ctx, w, status, editorID, fundID, &donations.FundNote{Body: body, Anonymous: anonymous}, message)

		return
	}

	// The editor again, holding what was stored. It is rendered on the thank-you
	// screen and on my donations, and both want the same thing afterwards: the
	// note as it now is, still editable.
	FundNoteForm(editorID, fundID, note, "", "your note is up on the fund's page.").Render(ctx, w)
}

// removeOwnFundNote takes down the note the signed-in donor wrote.
//
// Scoped to the member rather than taking a note id: a donor cannot name somebody
// else's note because there is nowhere to say which note, only which fund.
func (h *FundHandlers) removeOwnFundNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/login")

		return
	}

	fundID, err := uuid.Parse(r.PathValue("fundId"))
	if err != nil {
		h.badNote(ctx, w, http.StatusBadRequest, noteEditorID(fundID), fundID, nil, "that is not a fund")

		return
	}

	if err = h.donationService.RemoveOwnFundNote(ctx, fundID, member.ID); err != nil {
		h.logger.Error("failed to remove own fund note", slog.String("error", err.Error()))

		h.badNote(ctx, w, http.StatusInternalServerError, noteEditor(r, fundID), fundID, nil,
			"we could not take your note down. please try again.")

		return
	}

	// An empty editor: the note is gone and they can write another.
	FundNoteForm(noteEditor(r, fundID), fundID, nil, "", "your note has been taken down.").Render(ctx, w)
}

// noteFailure maps a refusal onto what the donor should be told. An empty message
// means it is not one of ours and the donor gets the generic apology.
func noteFailure(err error) (int, string) {
	switch {
	case errors.Is(err, donations.ErrNotADonor):
		return http.StatusForbidden, "only donors to this fund can leave a note."
	case errors.Is(err, donations.ErrNoteEmpty):
		return http.StatusBadRequest, "your note needs something in it."
	case errors.Is(err, donations.ErrNoteTooLong):
		return http.StatusBadRequest, donations.ErrNoteTooLong.Error() + "."
	default:
		return http.StatusInternalServerError, ""
	}
}

// badNote redraws the editor with what went wrong.
//
// A fragment, always. Every one of these is swapped into the form by
// hx-target-error, and a whole layout document dropped inside a form is not
// something a browser can make sense of: the section it lands in is the one that
// breaks, and the note the donor typed goes with it.
func (h *FundHandlers) badNote(ctx context.Context, w http.ResponseWriter, status int,
	editorID string, fundID uuid.UUID, attempt *donations.FundNote, message string) {
	w.WriteHeader(status)
	FundNoteForm(editorID, fundID, attempt, message, "").Render(ctx, w)
}

// noteEditor is which editor on the page this answer belongs to.
//
// The form sends its own element id back so the reply can wear it and remain
// swappable. A page can hold several, and the server has no other way to tell
// them apart. Falling back to the fund keeps a hand-made request working.
func noteEditor(r *http.Request, fundID uuid.UUID) string {
	if sent := r.FormValue("editor"); sent != "" {
		return sent
	}

	return noteEditorID(fundID)
}

// myDonations is a donor's own record, and the only place they can stop giving.
func (h *FundHandlers) myDonations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/login")

		return
	}

	donationsForMember, err := h.donationService.ListDonationsForMember(ctx, member.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	// Not fatal: the donations are what this page is for, and an editor that opens
	// empty is a smaller wrong than a page that will not load.
	notes, err := h.donationService.ListFundNotesForMember(ctx, member.ID)
	if err != nil {
		h.logger.Error("failed to list the member's notes", slog.String("error", err.Error()))
	}

	MyDonations(donationsForMember, notes, &member, r.URL.Path).Render(ctx, w)
}

// cancelMyDonation stops a recurring donation at the provider and then here.
//
// The member id comes from the session and never from the request, so the only
// donation a caller can name is one of their own -- the service refuses anything
// else, and refuses it as not-found so a member cannot learn which ids exist.
func (h *FundHandlers) cancelMyDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/login")

		return
	}

	donationID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, "that is not a donation id", "/donations", r.URL.Path).Render(ctx, w)

		return
	}

	err = h.donationService.CancelDonationForMember(ctx, donationID, member.ID)
	if err != nil {
		h.cancelFailed(ctx, w, r, member, donationID, err)

		return
	}

	// Re-read so the row redraws from what is now true rather than from what the
	// click assumed.
	h.renderDonationRow(ctx, w, member, donationID, "")
}

// cancelFailed reports a failed cancellation to the donor.
//
// Each case gets its own status and its own words. Reporting a one-off donation
// as a PayPal failure would be a lie about somebody's money, and telling a donor
// only "something went wrong" after they pressed cancel invites them to assume it
// worked -- which for a live subscription is the expensive assumption.
func (h *FundHandlers) cancelFailed(ctx context.Context, w http.ResponseWriter, r *http.Request,
	member members.Member, donationID uuid.UUID, err error) {
	switch {
	case errors.Is(err, donations.ErrDonationNotYours):
		// Not-found rather than forbidden: whether the id exists is not a caller's
		// business.
		h.renderCancelError(ctx, w, r, member, donationID,
			http.StatusNotFound, "no such donation")

	case errors.Is(err, donations.ErrDonationNotCancellable):
		// A one-off donation, or one already ended. No provider call was made and
		// none would have helped.
		h.renderCancelError(ctx, w, r, member, donationID,
			http.StatusBadRequest, "that donation cannot be cancelled")

	default:
		h.logger.Error("failed to cancel donation",
			slog.String("donation_id", donationID.String()),
			slog.String("error", err.Error()),
		)

		h.renderCancelError(ctx, w, r, member, donationID, http.StatusInternalServerError,
			"we could not stop this donation with PayPal, so it is still running. please try again.")
	}
}

// renderCancelError writes the failure where the donor will see it.
//
// A fragment for htmx, because the response is swapped into one row: rendering
// the full page shell there would nest a whole page inside a div. A full page for
// an ordinary navigation, which has nowhere to put a fragment.
func (h *FundHandlers) renderCancelError(ctx context.Context, w http.ResponseWriter, r *http.Request,
	member members.Member, donationID uuid.UUID, status int, message string) {
	if !common.IsHTMX(r) {
		w.WriteHeader(status)
		common.ErrorMessage(&member, message, "/donations", r.URL.Path).Render(ctx, w)

		return
	}

	w.WriteHeader(status)
	h.renderDonationRow(ctx, w, member, donationID, message)
}

// renderDonationRow redraws one donation, optionally carrying a failure.
//
// Re-read rather than reused, so the row reflects what the server did. When the
// donation cannot be found -- which is the not-yours case -- there is no row to
// draw, so the page reloads and shows the donor what they actually have.
func (h *FundHandlers) renderDonationRow(ctx context.Context, w http.ResponseWriter,
	member members.Member, donationID uuid.UUID, failure string) {
	forMember, err := h.donationService.ListDonationsForMember(ctx, member.ID)
	if err == nil {
		for _, donation := range forMember {
			if donation.ID == donationID {
				// The note comes along, because this row replaces itself: leaving it
				// out would make cancelling a donation quietly remove the control for
				// a note that is still up.
				note, errNote := h.donationService.GetFundNoteForMember(ctx, donation.FundID, member.ID)
				if errNote != nil {
					h.logger.Error("failed to read the member's note", slog.String("error", errNote.Error()))
				}

				MyDonationRow(donation, note, failure).Render(ctx, w)

				return
			}
		}
	}

	w.Header().Set("HX-Redirect", "/donations")
}

func (h *FundHandlers) ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func (h *FundHandlers) donationSuccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		// nil, not &member. The assertion failed, so member is the zero value, and
		// a non-nil pointer to it reads as logged in: the nav renders logout and
		// hides login, leaving the one link this visitor needs off the page.
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	// The fund the thank-you is for, so it can ask for a note about the thing just
	// given to. It comes from the query string and is therefore untrusted, which
	// costs nothing: SaveFundNote checks that this member gave to this fund, so a
	// forged id buys an editor whose every submission is refused.
	//
	// Asked for here rather than on the fund page. A form is an invitation, and on
	// the fund page it invited everybody and was accepted from almost nobody. Here
	// it is offered to exactly the person who earned it, at the moment they did.
	fundID, err := uuid.Parse(r.URL.Query().Get("fund"))
	if err != nil {
		// No fund named, so nothing to ask about. Still a thank-you.
		ThankYou(member, uuid.Nil, false, r.URL.Path).Render(ctx, w)

		return
	}

	canWrite, err := h.donationService.MemberHasGivenToFund(ctx, fundID, member.ID)
	if err != nil {
		h.logger.Error("failed to check whether the member has given", slog.String("error", err.Error()))
	}

	ThankYou(member, fundID, canWrite, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) completeOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		// nil, not &member. The assertion failed, so member is the zero value, and
		// a non-nil pointer to it reads as logged in: the nav renders logout and
		// hides login, leaving the one link this visitor needs off the page.
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund_id")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	orderID := r.FormValue("order_id")
	if orderID == "" {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	paymentID := r.FormValue("payment_id")
	if paymentID == "" {
		h.logger.Error("missing payment id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	// The caller's IP was normalised here and then dropped on the floor:
	// OneTimeCompletion.IPAddress was never assigned, and CompleteDonation does
	// not read it. Removed rather than wired up, since nothing stores it.
	completion := donations.OneTimeCompletion{
		AmountCents:       amountCents,
		FundID:            fundUUID,
		ProviderOrderID:   orderID,
		ProviderPaymentID: paymentID,
	}

	err = h.donationService.CompleteDonation(ctx, member.ID, completion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *FundHandlers) completeRecurringDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		// nil, not &member. The assertion failed, so member is the zero value, and
		// a non-nil pointer to it reads as logged in: the nav renders logout and
		// hides login, leaving the one link this visitor needs off the page.
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	planIDStr := r.FormValue("plan_id")
	if planIDStr == "" {
		h.logger.Error("missing plan id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	planUUID, err := uuid.Parse(planIDStr)
	if err != nil {
		h.logger.Error("unable to parse plan id", slog.String("plan_id", planIDStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundIDStr := r.FormValue("fund_id")
	if fundIDStr == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundIDStr)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundIDStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	providerSubscriptionID := r.FormValue("subscription_id")
	if providerSubscriptionID == "" {
		h.logger.Error("missing subscription id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	orderID := r.FormValue("order_id")
	if orderID == "" {
		h.logger.Error("missing order_id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	completion := donations.RecurringCompletion{
		PlanID: uuid.NullUUID{
			UUID:  planUUID,
			Valid: true,
		},
		AmountCents:            amountCents,
		FundID:                 fundUUID,
		ProviderOrderID:        orderID,
		ProviderSubscriptionID: providerSubscriptionID,
	}

	err = h.donationService.CompleteRecurringDonation(ctx, member.ID, completion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	// The browser discards this body and redirects to /donation/success, which is
	// where the note is asked for. Rendered anyway so the response is not empty,
	// but with nothing to ask: this is not a page anybody sees.
	ThankYou(member, fundUUID, false, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) createDonationPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	interval := r.FormValue("interval")
	if interval == "" {
		h.logger.Error("missing interval")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, "interval is required", "/", r.URL.Path).Render(ctx, w)

		return
	}
	amount := r.FormValue("amount")
	if amount == "" {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, "amount is required", "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountInt, err := strconv.Atoi(amount)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amount), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	plan := donations.CreatePlan{
		FundID:       fundUUID,
		Name:         fmt.Sprintf("%d-%s", amountInt, interval),
		AmountCents:  int32(amountInt * 100),
		IntervalUnit: donations.IntervalUnit(interval),
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	plan.ProviderFundID = fund.ProviderID

	newPlan, err := h.donationService.CreateDonationPlan(ctx, plan)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	PaypalSubscription(*newPlan, h.clientID, fund.Name).Render(ctx, w)
}

func (h *FundHandlers) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		// nil, not &member. The assertion failed, so member is the zero value, and
		// a non-nil pointer to it reads as logged in: the nav renders logout and
		// hides login, leaving the one link this visitor needs off the page.
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	funds, err := h.donationService.ListActiveFunds(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	closed, err := h.donationService.ListClosedFunds(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	// One query for every picture on the page, active and closed together. A
	// lookup per row would be a round trip per fund to draw the front page.
	//
	// Not fatal: this page is a list of funds, and it is still that without any
	// pictures on it.
	fundIDs := make([]uuid.UUID, 0, len(funds)+len(closed))
	for _, fund := range funds {
		fundIDs = append(fundIDs, fund.ID)
	}

	for _, fund := range closed {
		fundIDs = append(fundIDs, fund.ID)
	}

	images, err := h.donationService.GetFundImages(ctx, fundIDs)
	if err != nil {
		h.logger.Error("failed to read fund images", slog.String("error", err.Error()))
	}

	Funds(funds, closed, images, &member, r.URL.Path).Render(ctx, w)
}

// closedFundSummary is the archive page for one ended fund. A fund that is still
// open is redirected to its donation page rather than shown a summary that says
// it has closed.
func (h *FundHandlers) closedFundSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		// nil, not &member. The assertion failed, so member is the zero value, and
		// a non-nil pointer to it reads as logged in: the nav renders logout and
		// hides login, leaving the one link this visitor needs off the page.
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID, err := uuid.Parse(r.PathValue("fundId"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, "that is not a fund id", "/", r.URL.Path).Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetClosedFund(ctx, fundID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			common.ErrorMessage(&member, "no such fund", "/", r.URL.Path).Render(ctx, w)

			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	if !fund.Closed() {
		// Still taking donations, so the donation page is the honest destination.
		common.Redirect(w, r, "/donate/"+fundID.String())

		return
	}

	// Only the notes themselves. There is no form on this page to decide the
	// shape of, because a fund that has closed cannot be given to.
	notes, err := h.donationService.ListFundNotes(ctx, fundID)
	if err != nil {
		h.logger.Error("failed to list fund notes", slog.String("error", err.Error()))
	}

	image, err := h.donationService.GetFundImage(ctx, fundID)
	if err != nil {
		image = nil
	}

	ClosedFundSummary(*fund, fund.Stats, notes, image, &member, r.URL.Path).Render(ctx, w)
}

func sendJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func dollarStringToCents(dollars string) (int32, error) {
	dollars = strings.TrimSpace(dollars)

	decimalIndex := strings.Index(dollars, ".")

	if decimalIndex == -1 {
		dollars = dollars + ".00"
		decimalIndex = len(dollars) - 3
	}

	integerPart := dollars[:decimalIndex]
	fractionalPart := dollars[decimalIndex+1:]

	if len(fractionalPart) == 1 {
		fractionalPart += "0"
	} else if len(fractionalPart) > 2 {
		fractionalPart = fractionalPart[:2]
	}

	combinedAmount := integerPart + fractionalPart
	amountInCents, err := strconv.Atoi(combinedAmount)
	if err != nil {
		return 0, fmt.Errorf("invalid dollar amount format: %s", dollars)
	}

	return int32(amountInCents), nil
}

// fundImage serves a fund's picture.
//
// Public and unauthenticated, like the pages that show it: an image behind a
// login would render as a broken box for anybody not signed in.
//
// The hash in the path is part of the lookup, so this can only ever answer with
// the bytes its own URL identifies. That is what makes the long cache below safe
// at every layer: a stored copy cannot be of anything else, so replacing the
// image replaces the URL, and Cloudflare rewriting our max-age costs nothing.
func (h *FundHandlers) fundImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fundID, err := uuid.Parse(r.PathValue("fundId"))
	if err != nil {
		http.NotFound(w, r)

		return
	}

	body, object, err := h.donationService.OpenFundImage(ctx, fundID, r.PathValue("hash"))
	if err != nil {
		h.logger.Error("failed to read a fund image", slog.String("error", err.Error()))

		http.Error(w, "", http.StatusInternalServerError)

		return
	}

	if body == nil {
		http.NotFound(w, r)

		return
	}

	defer body.Close()

	// Our own content type, from what we re-encoded, and nosniff so a browser
	// cannot decide these bytes are something more interesting than a picture.
	w.Header().Set("Content-Type", object.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Last-Modified", object.Updated.UTC().Format(http.TimeFormat))

	// Copied rather than served with ServeContent: that wants a ReadSeeker so it
	// can answer range requests, and having one would mean holding the whole
	// picture in memory. Nothing asks for ranges of a photograph.
	if _, err = io.Copy(w, body); err != nil {
		// The header is long gone, so there is no status left to change. Logged
		// because a truncated picture otherwise looks like a browser problem.
		h.logger.Error("failed to write a fund image", slog.String("error", err.Error()))
	}
}
