package aws

import (
	"boardfund/service/auth"
	"context"
	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"time"
)

type Cognito interface {
	InitiateAuth(ctx context.Context, params *cognito.InitiateAuthInput, optFns ...func(*cognito.Options)) (*cognito.InitiateAuthOutput, error)
}

type CognitoAuth struct {
	awsCognito Cognito

	clientID string
}

func NewCognitoAuth(awsCognito Cognito, clientID string) *CognitoAuth {
	return &CognitoAuth{
		awsCognito: awsCognito,
		clientID:   clientID,
	}
}

func (c CognitoAuth) Authorize(ctx context.Context, user, pass string) (*auth.Token, error) {
	authResponse, err := c.awsCognito.InitiateAuth(ctx, &cognito.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeUserPasswordAuth,
		ClientId:       &c.clientID,
		AuthParameters: map[string]string{"USERNAME": user, "PASSWORD": pass},
	})
	if err != nil {
		return nil, err
	}

	expiration := time.Now().Add(time.Duration(authResponse.AuthenticationResult.ExpiresIn) * time.Second)

	return &auth.Token{
		TokenStr: *authResponse.AuthenticationResult.AccessToken,
		Expires:  expiration,
	}, nil
}
