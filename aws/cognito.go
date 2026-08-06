package aws

import (
	"boardfund/service/auth"
	"context"
	"errors"
	"fmt"
	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"log/slog"
	"time"
)

type awsCognito interface {
	InitiateAuth(ctx context.Context, params *cognito.InitiateAuthInput, optFns ...func(*cognito.Options)) (*cognito.InitiateAuthOutput, error)
	AdminCreateUser(ctx context.Context, params *cognito.AdminCreateUserInput, optFns ...func(*cognito.Options)) (*cognito.AdminCreateUserOutput, error)
	AdminDeleteUser(ctx context.Context, params *cognito.AdminDeleteUserInput, optFns ...func(*cognito.Options)) (*cognito.AdminDeleteUserOutput, error)
	AdminSetUserPassword(ctx context.Context, params *cognito.AdminSetUserPasswordInput, optFns ...func(*cognito.Options)) (*cognito.AdminSetUserPasswordOutput, error)
	AdminAddUserToGroup(ctx context.Context, params *cognito.AdminAddUserToGroupInput, optFns ...func(*cognito.Options)) (*cognito.AdminAddUserToGroupOutput, error)
	AdminRemoveUserFromGroup(ctx context.Context, params *cognito.AdminRemoveUserFromGroupInput, optFns ...func(*cognito.Options)) (*cognito.AdminRemoveUserFromGroupOutput, error)
	AdminListGroupsForUser(ctx context.Context, params *cognito.AdminListGroupsForUserInput, optFns ...func(*cognito.Options)) (*cognito.AdminListGroupsForUserOutput, error)
}

type CognitoAuth struct {
	awsCognito awsCognito

	logger *slog.Logger

	clientID   string
	userPoolID string
}

func NewCognitoAuth(awsCognito awsCognito, logger *slog.Logger, clientID, userPoolID string) *CognitoAuth {
	return &CognitoAuth{
		awsCognito: awsCognito,
		logger:     logger,
		clientID:   clientID,
		userPoolID: userPoolID,
	}
}

func (c CognitoAuth) Authorize(ctx context.Context, user, pass string) (*auth.AuthResponse, error) {
	initiateResponse, err := c.awsCognito.InitiateAuth(ctx, &cognito.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeUserPasswordAuth,
		ClientId:       &c.clientID,
		AuthParameters: map[string]string{"USERNAME": user, "PASSWORD": pass},
	})
	if err != nil {
		c.logger.Error("failed to auth", slog.String("error", err.Error()))

		return nil, handleCognitoError(err, auth.ErrAuthenticateOther)
	}

	if initiateResponse.AuthenticationResult == nil {
		if initiateResponse.ChallengeName == types.ChallengeNameTypeNewPasswordRequired {
			return &auth.AuthResponse{
				ResetPassword: true,
			}, nil
		}

		return nil, fmt.Errorf("could not authenticate")
	}

	expiration := time.Now().Add(time.Duration(initiateResponse.AuthenticationResult.ExpiresIn) * time.Second)

	return &auth.AuthResponse{
		Token: &auth.Token{
			AccessTokenStr: *initiateResponse.AuthenticationResult.AccessToken,
			IDTokenStr:     *initiateResponse.AuthenticationResult.IdToken,
			Expires:        expiration,
		},
	}, nil
}

func (c CognitoAuth) SetPassword(ctx context.Context, user, old, new string) error {
	_, err := c.awsCognito.InitiateAuth(ctx, &cognito.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeUserPasswordAuth,
		ClientId:       &c.clientID,
		AuthParameters: map[string]string{"USERNAME": user, "PASSWORD": old},
	})
	if err != nil {
		c.logger.Error("failed to auth", slog.String("error", err.Error()))

		return handleCognitoError(err, auth.ErrAuthenticateOther)
	}

	_, err = c.awsCognito.AdminSetUserPassword(ctx, &cognito.AdminSetUserPasswordInput{
		UserPoolId: &c.userPoolID,
		Username:   &user,
		Password:   &new,
		Permanent:  true,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c CognitoAuth) CreateUser(ctx context.Context, username, email string, memberID uuid.UUID) (string, error) {
	resp, err := c.awsCognito.AdminCreateUser(ctx, &cognito.AdminCreateUserInput{
		UserPoolId: &c.userPoolID,
		Username:   &username,
		DesiredDeliveryMediums: []types.DeliveryMediumType{
			types.DeliveryMediumTypeEmail,
		},
		UserAttributes: []types.AttributeType{
			{
				Name:  toPointer("custom:member_id"),
				Value: toPointer(memberID.String()),
			},
			{
				Name:  toPointer("email"),
				Value: toPointer(email),
			},
		},
	})
	if err != nil {
		return "", handleCognitoError(err, auth.ErrNewUserOther)
	}

	var cognitoID string
	for _, attr := range resp.User.Attributes {
		if *attr.Name == "sub" {
			cognitoID = *attr.Value
		}
	}

	return cognitoID, nil
}

func (c CognitoAuth) DeleteUser(ctx context.Context, username string) error {
	_, err := c.awsCognito.AdminDeleteUser(ctx, &cognito.AdminDeleteUserInput{
		UserPoolId: &c.userPoolID,
		Username:   &username,
	})
	if err != nil {
		return err
	}

	return nil
}

// AddToGroup is idempotent at the provider: adding a user already in the group
// succeeds rather than erroring, so a double-click promotes once.
func (c CognitoAuth) AddToGroup(ctx context.Context, username, group string) error {
	_, err := c.awsCognito.AdminAddUserToGroup(ctx, &cognito.AdminAddUserToGroupInput{
		UserPoolId: &c.userPoolID,
		Username:   &username,
		GroupName:  &group,
	})
	if err != nil {
		c.logger.Error("failed to add user to group",
			slog.String("group", group),
			slog.String("error", err.Error()),
		)

		return handleCognitoError(err, auth.ErrGroupOther)
	}

	return nil
}

// RemoveFromGroup is likewise idempotent: removing a user who is not in the
// group is not an error.
func (c CognitoAuth) RemoveFromGroup(ctx context.Context, username, group string) error {
	_, err := c.awsCognito.AdminRemoveUserFromGroup(ctx, &cognito.AdminRemoveUserFromGroupInput{
		UserPoolId: &c.userPoolID,
		Username:   &username,
		GroupName:  &group,
	})
	if err != nil {
		c.logger.Error("failed to remove user from group",
			slog.String("group", group),
			slog.String("error", err.Error()),
		)

		return handleCognitoError(err, auth.ErrGroupOther)
	}

	return nil
}

// ListGroups returns every group the user belongs to. It follows the pagination
// token rather than reading the first page: a truncated reply would report an
// admin as not being one, which reads as the promotion having failed.
func (c CognitoAuth) ListGroups(ctx context.Context, username string) ([]string, error) {
	var groups []string
	var next *string

	for {
		resp, err := c.awsCognito.AdminListGroupsForUser(ctx, &cognito.AdminListGroupsForUserInput{
			UserPoolId: &c.userPoolID,
			Username:   &username,
			NextToken:  next,
		})
		if err != nil {
			c.logger.Error("failed to list groups for user", slog.String("error", err.Error()))

			return nil, handleCognitoError(err, auth.ErrGroupOther)
		}

		for _, group := range resp.Groups {
			if group.GroupName != nil {
				groups = append(groups, *group.GroupName)
			}
		}

		if resp.NextToken == nil || *resp.NextToken == "" {
			return groups, nil
		}

		next = resp.NextToken
	}
}

func handleCognitoError(err, base error) error {
	var target smithy.APIError
	if !errors.As(err, &target) {
		return base
	}

	switch target.ErrorCode() {
	case "UserNotFoundException":
		return auth.ErrUserNotFound
	case "NotAuthorizedException":
		return auth.ErrInvalidCredentials
	case "UsernameExistsException":
		return auth.ErrUsernameExists
	case "InvalidPasswordException":
		return auth.ErrInvalidPassword
	default:
		return base
	}
}
