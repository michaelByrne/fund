package memberweb

import (
	"boardfund/service/members"
	"boardfund/web/mux"
	"encoding/json"
	"github.com/alexedwards/scs/v2"
	"io"
	"net/http"
	"strings"
)

type MemberHandler struct {
	memberService *members.MemberService
	sessionStore  *scs.SessionManager
	withAuth      func(next http.HandlerFunc) http.HandlerFunc
}

func NewMemberHandler(memberService *members.MemberService, sessionStore *scs.SessionManager, withAuth func(next http.HandlerFunc) http.HandlerFunc) *MemberHandler {
	return &MemberHandler{
		memberService: memberService,
		sessionStore:  sessionStore,
		withAuth:      withAuth,
	}
}

func (m *MemberHandler) Register(r *mux.Router) {
	r.HandleFunc("/member", m.createMember)
}

func (m *MemberHandler) createMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	body := r.Body
	defer body.Close()

	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return
	}

	var newMember members.Member
	err = json.Unmarshal(bodyBytes, &newMember)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))

		return
	}

	ipAddress := r.RemoteAddr
	if strings.Contains(ipAddress, "[::1]") {
		ipAddress = "127.0.0.1"
	}

	splitIP := strings.Split(ipAddress, ":")
	if len(splitIP) > 1 {
		ipAddress = splitIP[0]
	}

	newMember.IPAddress = ipAddress

	createdMember, err := m.memberService.CreateMember(ctx, newMember)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return
	}

	createdMemberBytes, err := json.Marshal(createdMember)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write(createdMemberBytes)
}
