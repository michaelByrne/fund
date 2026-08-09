package auth

import (
	"boardfund/jwtauth"
	"boardfund/service/adminevents"
	"boardfund/service/members"
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"log/slog"
)

type memberStore interface {
	GetMemberByID(ctx context.Context, id uuid.UUID) (*members.Member, error)
	UpsertMember(ctx context.Context, upsert members.UpsertMember) (*members.Member, error)
}

type authStore interface {
	InsertApprovedEmail(ctx context.Context, arg string) (*ApprovedEmail, error)
	GetApprovedEmail(ctx context.Context, arg string) (*ApprovedEmail, error)
	MarkEmailAsUsed(ctx context.Context, email string) (*ApprovedEmail, error)
	GetApprovedEmails(ctx context.Context) ([]ApprovedEmail, error)
	DeleteApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error)
}

// adminEventRecorder is the audit trail for privilege changes. Narrowed to the
// one method used so the service can be exercised without a database.
type adminEventRecorder interface {
	Record(ctx context.Context, record adminevents.Record)
}

type authorizer interface {
	Authorize(ctx context.Context, user, pass string) (*AuthResponse, error)
	SetPassword(ctx context.Context, user, old, new string) error
	CreateUser(ctx context.Context, username, email string, memberID uuid.UUID) (string, error)
	AddToGroup(ctx context.Context, username, group string) error
	RemoveFromGroup(ctx context.Context, username, group string) error
	ListGroups(ctx context.Context, username string) ([]string, error)
}

type AuthService struct {
	memberStore memberStore
	authStore   authStore
	authorizer  authorizer
	adminEvents adminEventRecorder

	logger *slog.Logger
}

func NewAuthService(memberStore memberStore, authStore authStore, authorizer authorizer, adminEvents adminEventRecorder, logger *slog.Logger) *AuthService {
	return &AuthService{
		memberStore: memberStore,
		authStore:   authStore,
		authorizer:  authorizer,
		adminEvents: adminEvents,
		logger:      logger,
	}
}

func (s AuthService) Register(ctx context.Context, username, email string) (*members.Member, error) {
	memberID := uuid.New()

	cognitoID, err := s.authorizer.CreateUser(ctx, username, email, memberID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to create user", slog.String("error", err.Error()))

		return nil, err
	}

	upsert := members.UpsertMember{
		ID:        memberID,
		CognitoID: cognitoID,
		Email:     email,
		BCOName:   username,
	}

	member, err := s.memberStore.UpsertMember(ctx, upsert)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to upsert member", slog.String("error", err.Error()))

		return nil, err
	}

	return member, nil
}

// GrantAdmin puts a member in the Cognito group that authorises the admin
// section, which is the whole of what makes someone an admin -- see the comment
// on jwtauth.AdminGroup. Nothing is written to member.roles: that column
// authorises nothing, and Authenticate already derives the session's view of it
// from the token, so a second copy could only ever disagree.
//
// Cognito stamps group membership into the ID token at authentication, so the
// member stays non-admin until they log in again. Callers should say so.
//
// It takes both members rather than the subject's username because the audit
// record needs the actor, and a parameter that is easy to omit is one that gets
// omitted. Cognito is addressed by username; the log is written in member ids.
func (s AuthService) GrantAdmin(ctx context.Context, actor, subject members.Member) error {
	if err := s.authorizer.AddToGroup(ctx, subject.BCOName, jwtauth.AdminGroup); err != nil {
		s.logger.ErrorContext(ctx, "failed to grant admin",
			slog.String("username", subject.BCOName),
			slog.String("error", err.Error()),
		)

		return err
	}

	s.recordAdminChange(ctx, adminevents.KindAdminGranted, actor, subject)

	return nil
}

// RevokeAdmin is the inverse. It takes effect on the member's next login for the
// same reason, but sooner in practice: their current token expires within the
// hour, and nothing reissues one without a fresh authentication.
func (s AuthService) RevokeAdmin(ctx context.Context, actor, subject members.Member) error {
	if err := s.authorizer.RemoveFromGroup(ctx, subject.BCOName, jwtauth.AdminGroup); err != nil {
		s.logger.ErrorContext(ctx, "failed to revoke admin",
			slog.String("username", subject.BCOName),
			slog.String("error", err.Error()),
		)

		return err
	}

	s.recordAdminChange(ctx, adminevents.KindAdminRevoked, actor, subject)

	return nil
}

// recordAdminChange writes the audit line, after the group write has succeeded
// and never before it: an event recorded ahead of the change it describes is a
// claim that something happened when it may not have.
//
// This is also the log line. There was a separate one here saying "granted
// admin" with the member's bco_name on it; adminevents.Record now writes the
// same fact with member ids instead, so the duplicate went rather than being
// copied. The error paths above still name the username, because that is the key
// the failing Cognito call was made with and is what a person would need to
// retry it by hand.
func (s AuthService) recordAdminChange(ctx context.Context, kind adminevents.Kind, actor, subject members.Member) {
	if s.adminEvents == nil {
		return
	}

	// No Detail: every change made here is the same change to the same Cognito
	// group, so a constant string in every row would be a column the reader
	// learns to skip. It is left for the cases that differ.
	record := adminevents.Record{
		Kind:            kind,
		SubjectMemberID: subject.ID,
	}

	// A zero id means the caller had no signed-in member to attribute this to,
	// which the log shows as unattributed rather than as member 000...0.
	if actor.ID != uuid.Nil {
		id := actor.ID
		record.ActorMemberID = &id
	}

	s.adminEvents.Record(ctx, record)
}

// IsAdmin asks Cognito rather than the database. member.roles would be cheaper to
// read and wrong: nothing writes ADMIN to it, so it reports every admin as an
// ordinary member.
func (s AuthService) IsAdmin(ctx context.Context, username string) (bool, error) {
	groups, err := s.authorizer.ListGroups(ctx, username)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to list groups",
			slog.String("username", username),
			slog.String("error", err.Error()),
		)

		return false, err
	}

	for _, group := range groups {
		if group == jwtauth.AdminGroup {
			return true, nil
		}
	}

	return false, nil
}

func (s AuthService) ResetPassword(ctx context.Context, username, password, newPassword string) (*members.Member, *AuthResponse, error) {
	err := s.authorizer.SetPassword(ctx, username, password, newPassword)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to reset password", slog.String("error", err.Error()))

		return nil, nil, err
	}

	member, autResp, err := s.Authenticate(ctx, username, newPassword)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to authenticate", slog.String("error", err.Error()))

		return nil, nil, err
	}

	return member, autResp, nil
}

func (s AuthService) GetApprovedEmails(ctx context.Context) ([]ApprovedEmail, error) {
	emails, err := s.authStore.GetApprovedEmails(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get approved emails", slog.String("error", err.Error()))

		return nil, err
	}

	return emails, nil
}

func (s AuthService) GetApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.GetApprovedEmail(ctx, email)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get approved email", slog.String("error", err.Error()))

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEmailNotApproved
		}

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) MarkEmailAsUsed(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.MarkEmailAsUsed(ctx, email)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to mark email as used", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) InsertApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.InsertApprovedEmail(ctx, email)
	if err != nil {
		// approved_email.email is the primary key, so re-adding an address is a
		// unique violation. That is an operator repeating themselves, not a fault.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, ErrEmailAlreadyApproved
		}

		s.logger.ErrorContext(ctx, "failed to insert approved email", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) DeleteApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.DeleteApprovedEmail(ctx, email)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to delete approved email", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) Authenticate(ctx context.Context, username, password string) (*members.Member, *AuthResponse, error) {
	resp, err := s.authorizer.Authorize(ctx, username, password)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to authenticate", slog.String("error", err.Error()))

		return nil, nil, err
	}

	if resp.ResetPassword {
		return nil, resp, nil
	}

	parsedToken, err := jwt.ParseString(resp.Token.IDTokenStr, jwt.WithVerify(false))
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to parse token", slog.String("error", err.Error()))

		return nil, nil, err
	}

	claims := parsedToken.PrivateClaims()
	memberID := claims["custom:member_id"].(string)

	memberUUID, err := uuid.Parse(memberID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to parse member id", slog.String("error", err.Error()))

		return nil, nil, err
	}

	member, err := s.memberStore.GetMemberByID(ctx, memberUUID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get member by id", slog.String("error", err.Error()))

		return nil, nil, err
	}

	// Admin routes are gated on the token's Cognito group, but the navigation asks
	// member.IsAdmin(), which reads member.roles -- a column nothing ever writes
	// ADMIN to. Left alone, the admin section is invisible to every admin.
	//
	// Reconcile them here rather than in the database: the token is what actually
	// authorises the request, so deriving the session's view from it means the menu
	// cannot disagree with what the middleware will allow.
	if jwtauth.HasGroup(claims, jwtauth.AdminGroup) && !member.IsAdmin() {
		member.Roles = append(member.Roles, members.AdminRole)
	}

	return member, resp, nil
}
