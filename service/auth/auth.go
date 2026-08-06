package auth

import (
	"boardfund/jwtauth"
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

	logger *slog.Logger
}

func NewAuthService(memberStore memberStore, authStore authStore, authorizer authorizer, logger *slog.Logger) *AuthService {
	return &AuthService{
		memberStore: memberStore,
		authStore:   authStore,
		authorizer:  authorizer,
		logger:      logger,
	}
}

func (s AuthService) Register(ctx context.Context, username, email string) (*members.Member, error) {
	memberID := uuid.New()

	cognitoID, err := s.authorizer.CreateUser(ctx, username, email, memberID)
	if err != nil {
		s.logger.Error("failed to create user", slog.String("error", err.Error()))

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
		s.logger.Error("failed to upsert member", slog.String("error", err.Error()))

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
func (s AuthService) GrantAdmin(ctx context.Context, username string) error {
	if err := s.authorizer.AddToGroup(ctx, username, jwtauth.AdminGroup); err != nil {
		s.logger.Error("failed to grant admin",
			slog.String("username", username),
			slog.String("error", err.Error()),
		)

		return err
	}

	s.logger.Info("granted admin", slog.String("username", username))

	return nil
}

// RevokeAdmin is the inverse. It takes effect on the member's next login for the
// same reason, but sooner in practice: their current token expires within the
// hour, and nothing reissues one without a fresh authentication.
func (s AuthService) RevokeAdmin(ctx context.Context, username string) error {
	if err := s.authorizer.RemoveFromGroup(ctx, username, jwtauth.AdminGroup); err != nil {
		s.logger.Error("failed to revoke admin",
			slog.String("username", username),
			slog.String("error", err.Error()),
		)

		return err
	}

	s.logger.Info("revoked admin", slog.String("username", username))

	return nil
}

// IsAdmin asks Cognito rather than the database. member.roles would be cheaper to
// read and wrong: nothing writes ADMIN to it, so it reports every admin as an
// ordinary member.
func (s AuthService) IsAdmin(ctx context.Context, username string) (bool, error) {
	groups, err := s.authorizer.ListGroups(ctx, username)
	if err != nil {
		s.logger.Error("failed to list groups",
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
		s.logger.Error("failed to reset password", slog.String("error", err.Error()))

		return nil, nil, err
	}

	member, autResp, err := s.Authenticate(ctx, username, newPassword)
	if err != nil {
		s.logger.Error("failed to authenticate", slog.String("error", err.Error()))

		return nil, nil, err
	}

	return member, autResp, nil
}

func (s AuthService) GetApprovedEmails(ctx context.Context) ([]ApprovedEmail, error) {
	emails, err := s.authStore.GetApprovedEmails(ctx)
	if err != nil {
		s.logger.Error("failed to get approved emails", slog.String("error", err.Error()))

		return nil, err
	}

	return emails, nil
}

func (s AuthService) GetApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.GetApprovedEmail(ctx, email)
	if err != nil {
		s.logger.Error("failed to get approved email", slog.String("error", err.Error()))

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
		s.logger.Error("failed to mark email as used", slog.String("error", err.Error()))

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

		s.logger.Error("failed to insert approved email", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) DeleteApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.DeleteApprovedEmail(ctx, email)
	if err != nil {
		s.logger.Error("failed to delete approved email", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) Authenticate(ctx context.Context, username, password string) (*members.Member, *AuthResponse, error) {
	resp, err := s.authorizer.Authorize(ctx, username, password)
	if err != nil {
		s.logger.Error("failed to authenticate", slog.String("error", err.Error()))

		return nil, nil, err
	}

	if resp.ResetPassword {
		return nil, resp, nil
	}

	parsedToken, err := jwt.ParseString(resp.Token.IDTokenStr, jwt.WithVerify(false))
	if err != nil {
		s.logger.Error("failed to parse token", slog.String("error", err.Error()))

		return nil, nil, err
	}

	claims := parsedToken.PrivateClaims()
	memberID := claims["custom:member_id"].(string)

	memberUUID, err := uuid.Parse(memberID)
	if err != nil {
		s.logger.Error("failed to parse member id", slog.String("error", err.Error()))

		return nil, nil, err
	}

	member, err := s.memberStore.GetMemberByID(ctx, memberUUID)
	if err != nil {
		s.logger.Error("failed to get member by id", slog.String("error", err.Error()))

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
