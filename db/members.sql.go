// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0
// source: members.sql

package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const getMemberById = `-- name: GetMemberById :one
SELECT id, first_name, last_name, bco_name, roles, email, ip_address, last_login, cognito_id, paypal_email, postal_code, created, updated, provider_payer_id
FROM member
WHERE id = $1
`

func (q *Queries) GetMemberById(ctx context.Context, id uuid.UUID) (Member, error) {
	row := q.db.QueryRow(ctx, getMemberById, id)
	var i Member
	err := row.Scan(
		&i.ID,
		&i.FirstName,
		&i.LastName,
		&i.BcoName,
		&i.Roles,
		&i.Email,
		&i.IpAddress,
		&i.LastLogin,
		&i.CognitoID,
		&i.PaypalEmail,
		&i.PostalCode,
		&i.Created,
		&i.Updated,
		&i.ProviderPayerID,
	)
	return i, err
}

const upsertMember = `-- name: UpsertMember :one
INSERT INTO member (id, bco_name, email, cognito_id, first_name, last_name, provider_payer_id, roles)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE
    SET bco_name          = $2,
        email             = $3,
        cognito_id        = $4,
        first_name        = $5,
        last_name         = $6,
        provider_payer_id = $7,
        roles             = $8,
        updated           = now()
RETURNING id, first_name, last_name, bco_name, roles, email, ip_address, last_login, cognito_id, paypal_email, postal_code, created, updated, provider_payer_id
`

type UpsertMemberParams struct {
	ID              uuid.UUID
	BcoName         pgtype.Text
	Email           string
	CognitoID       pgtype.Text
	FirstName       pgtype.Text
	LastName        pgtype.Text
	ProviderPayerID pgtype.Text
	Roles           []Role
}

func (q *Queries) UpsertMember(ctx context.Context, arg UpsertMemberParams) (Member, error) {
	row := q.db.QueryRow(ctx, upsertMember,
		arg.ID,
		arg.BcoName,
		arg.Email,
		arg.CognitoID,
		arg.FirstName,
		arg.LastName,
		arg.ProviderPayerID,
		arg.Roles,
	)
	var i Member
	err := row.Scan(
		&i.ID,
		&i.FirstName,
		&i.LastName,
		&i.BcoName,
		&i.Roles,
		&i.Email,
		&i.IpAddress,
		&i.LastLogin,
		&i.CognitoID,
		&i.PaypalEmail,
		&i.PostalCode,
		&i.Created,
		&i.Updated,
		&i.ProviderPayerID,
	)
	return i, err
}
