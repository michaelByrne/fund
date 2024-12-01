// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0
// source: members.sql

package db

import (
	"context"
	"net/netip"

	"github.com/jackc/pgx/v5/pgtype"
)

const getMemberById = `-- name: GetMemberById :one
SELECT id, first_name, last_name, bco_name, ip_address, paypal_email, postal_code, created, updated, provider_payer_id
FROM member
WHERE id = $1
`

func (q *Queries) GetMemberById(ctx context.Context, id int32) (Member, error) {
	row := q.db.QueryRow(ctx, getMemberById, id)
	var i Member
	err := row.Scan(
		&i.ID,
		&i.FirstName,
		&i.LastName,
		&i.BcoName,
		&i.IpAddress,
		&i.PaypalEmail,
		&i.PostalCode,
		&i.Created,
		&i.Updated,
		&i.ProviderPayerID,
	)
	return i, err
}

const getMemberByPaypalEmail = `-- name: GetMemberByPaypalEmail :one
SELECT id, first_name, last_name, bco_name, ip_address, paypal_email, postal_code, created, updated, provider_payer_id
FROM member
WHERE paypal_email = $1
`

func (q *Queries) GetMemberByPaypalEmail(ctx context.Context, paypalEmail string) (Member, error) {
	row := q.db.QueryRow(ctx, getMemberByPaypalEmail, paypalEmail)
	var i Member
	err := row.Scan(
		&i.ID,
		&i.FirstName,
		&i.LastName,
		&i.BcoName,
		&i.IpAddress,
		&i.PaypalEmail,
		&i.PostalCode,
		&i.Created,
		&i.Updated,
		&i.ProviderPayerID,
	)
	return i, err
}

const insertMember = `-- name: InsertMember :one
INSERT INTO member (bco_name, ip_address, paypal_email, first_name, last_name, provider_payer_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, first_name, last_name, bco_name, ip_address, paypal_email, postal_code, created, updated, provider_payer_id
`

type InsertMemberParams struct {
	BcoName         pgtype.Text
	IpAddress       netip.Addr
	PaypalEmail     string
	FirstName       pgtype.Text
	LastName        pgtype.Text
	ProviderPayerID pgtype.Text
}

func (q *Queries) InsertMember(ctx context.Context, arg InsertMemberParams) (Member, error) {
	row := q.db.QueryRow(ctx, insertMember,
		arg.BcoName,
		arg.IpAddress,
		arg.PaypalEmail,
		arg.FirstName,
		arg.LastName,
		arg.ProviderPayerID,
	)
	var i Member
	err := row.Scan(
		&i.ID,
		&i.FirstName,
		&i.LastName,
		&i.BcoName,
		&i.IpAddress,
		&i.PaypalEmail,
		&i.PostalCode,
		&i.Created,
		&i.Updated,
		&i.ProviderPayerID,
	)
	return i, err
}

const upsertMember = `-- name: UpsertMember :one
INSERT INTO member (bco_name, ip_address, paypal_email, first_name, last_name, provider_payer_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (paypal_email) DO UPDATE
SET bco_name = $1, ip_address = $2, first_name = $4, last_name = $5, provider_payer_id = $6
RETURNING id, first_name, last_name, bco_name, ip_address, paypal_email, postal_code, created, updated, provider_payer_id
`

type UpsertMemberParams struct {
	BcoName         pgtype.Text
	IpAddress       netip.Addr
	PaypalEmail     string
	FirstName       pgtype.Text
	LastName        pgtype.Text
	ProviderPayerID pgtype.Text
}

func (q *Queries) UpsertMember(ctx context.Context, arg UpsertMemberParams) (Member, error) {
	row := q.db.QueryRow(ctx, upsertMember,
		arg.BcoName,
		arg.IpAddress,
		arg.PaypalEmail,
		arg.FirstName,
		arg.LastName,
		arg.ProviderPayerID,
	)
	var i Member
	err := row.Scan(
		&i.ID,
		&i.FirstName,
		&i.LastName,
		&i.BcoName,
		&i.IpAddress,
		&i.PaypalEmail,
		&i.PostalCode,
		&i.Created,
		&i.Updated,
		&i.ProviderPayerID,
	)
	return i, err
}
