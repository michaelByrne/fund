package aws

import (
	"boardfund/service/auth"
	"context"
	"fmt"
	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/google/uuid"
	"time"
)

type Cognito interface {
	InitiateAuth(ctx context.Context, params *cognito.InitiateAuthInput, optFns ...func(*cognito.Options)) (*cognito.InitiateAuthOutput, error)
	AdminCreateUser(ctx context.Context, params *cognito.AdminCreateUserInput, optFns ...func(*cognito.Options)) (*cognito.AdminCreateUserOutput, error)
	AdminDeleteUser(ctx context.Context, params *cognito.AdminDeleteUserInput, optFns ...func(*cognito.Options)) (*cognito.AdminDeleteUserOutput, error)
	AdminSetUserPassword(ctx context.Context, params *cognito.AdminSetUserPasswordInput, optFns ...func(*cognito.Options)) (*cognito.AdminSetUserPasswordOutput, error)
}

type CognitoAuth struct {
	awsCognito Cognito

	clientID   string
	userPoolID string
}

func NewCognitoAuth(awsCognito Cognito, clientID, userPoolID string) *CognitoAuth {
	return &CognitoAuth{
		awsCognito: awsCognito,
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
		return nil, err
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
		return err
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
		return "", err
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

func toPointer[T any](v T) *T {
	return &v
}
