package store

import (
	"time"

	"boardfund/db"
	"boardfund/service/payouts"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()}
}

func nullDBTime(t *time.Time) db.NullDBTime {
	if t == nil {
		return db.NullDBTime{}
	}

	return db.NullDBTime{DBTime: db.DBTime{Time: *t}, Valid: true}
}

func timePtr(t db.NullDBTime) *time.Time {
	if !t.Valid || t.Time.IsZero() {
		return nil
	}

	out := t.Time

	return &out
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}

	out := id.UUID

	return &out
}

func nullUUID(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}

func toDBInsertBatchParams(arg payouts.InsertBatch) db.InsertBatchPayoutParams {
	return db.InsertBatchPayoutParams{
		ID:               arg.ID,
		FundID:           arg.FundID,
		AmountCents:      arg.AmountCents,
		NumEnrollments:   arg.NumEnrollments,
		Status:           db.PayoutStatus(arg.Status),
		Description:      text(arg.Description),
		Notes:            text(arg.Notes),
		PayoutDate:       timestamptz(arg.PayoutDate),
		SenderBatchID:    arg.SenderBatchID,
		ApprovalDeadline: nullDBTime(arg.ApprovalDeadline),
	}
}

func fromDBBatch(dbBatch db.BatchPayout) payouts.Batch {
	return payouts.Batch{
		ID:               dbBatch.ID,
		FundID:           dbBatch.FundID,
		SenderBatchID:    dbBatch.SenderBatchID,
		ProviderBatchID:  dbBatch.ProviderBatchID.String,
		AmountCents:      dbBatch.AmountCents,
		NumEnrollments:   dbBatch.NumEnrollments,
		Status:           payouts.Status(dbBatch.Status),
		FailureReason:    dbBatch.FailureReason.String,
		Notes:            dbBatch.Notes.String,
		Description:      dbBatch.Description.String,
		PayoutDate:       dbBatch.PayoutDate.Time,
		ApprovalDeadline: timePtr(dbBatch.ApprovalDeadline),
		ApprovedBy:       uuidPtr(dbBatch.ApprovedBy),
		ApprovedAt:       timePtr(dbBatch.ApprovedAt),
		ReminderSentAt:   timePtr(dbBatch.ReminderSentAt),
		Created:          dbBatch.Created.Time,
		Updated:          dbBatch.Updated.Time,
	}
}

func fromDBDetailedBatch(row db.GetDetailedBatchPayoutsByStatusRow) payouts.BatchDetail {
	return payouts.BatchDetail{
		Batch: payouts.Batch{
			ID:               row.ID,
			FundID:           row.FundID,
			SenderBatchID:    row.SenderBatchID,
			ProviderBatchID:  row.ProviderBatchID.String,
			AmountCents:      row.AmountCents,
			NumEnrollments:   row.NumEnrollments,
			Status:           payouts.Status(row.Status),
			FailureReason:    row.FailureReason.String,
			Notes:            row.Notes.String,
			Description:      row.Description.String,
			PayoutDate:       row.PayoutDate.Time,
			ApprovalDeadline: timePtr(row.ApprovalDeadline),
			ApprovedBy:       uuidPtr(row.ApprovedBy),
			ApprovedAt:       timePtr(row.ApprovedAt),
			ReminderSentAt:   timePtr(row.ReminderSentAt),
			Created:          row.Created.Time,
			Updated:          row.Updated.Time,
		},
		FundName:   row.FundName,
		PayeeNames: row.PayeeNames,
	}
}

func toDBInsertPayoutParams(arg payouts.InsertPayout) db.InsertPayoutParams {
	return db.InsertPayoutParams{
		ID:               arg.ID,
		FundEnrollmentID: arg.FundEnrollmentID,
		BatchID:          arg.BatchID,
		AmountCents:      arg.AmountCents,
		Status:           db.PayoutStatus(arg.Status),
		Description:      text(arg.Description),
		Notes:            text(arg.Notes),
		PayoutDate:       timestamptz(arg.PayoutDate),
		DestinationEmail: arg.DestinationEmail,
	}
}

func fromDBPayout(dbPayout db.Payout) payouts.Payout {
	return payouts.Payout{
		ID:                   dbPayout.ID,
		FundEnrollmentID:     dbPayout.FundEnrollmentID,
		BatchID:              dbPayout.BatchID,
		ProviderPayoutItemID: dbPayout.ProviderPayoutItemID.String,
		AmountCents:          dbPayout.AmountCents,
		ProviderFeeCents:     dbPayout.ProviderFeeCents,
		DestinationEmail:     dbPayout.DestinationEmail,
		Status:               payouts.Status(dbPayout.Status),
		FailureReason:        dbPayout.FailureReason.String,
		Notes:                dbPayout.Notes.String,
		Description:          dbPayout.Description.String,
		PayoutDate:           dbPayout.PayoutDate.Time,
		Created:              dbPayout.Created.Time,
		Updated:              dbPayout.Updated.Time,
	}
}

func fromDBPayoutEnrollment(dbEnrollment db.FundEnrollment) payouts.PayoutEnrollment {
	return payouts.PayoutEnrollment{
		ID:              dbEnrollment.ID,
		FundID:          dbEnrollment.FundID,
		MemberID:        dbEnrollment.MemberID,
		MemberBCOName:   dbEnrollment.MemberBcoName.String,
		PaypalEmail:     dbEnrollment.PaypalEmail,
		FirstPayoutDate: dbEnrollment.FirstPayoutDate.Time,
		Created:         dbEnrollment.Created.Time,
		Updated:         dbEnrollment.Updated.Time,
	}
}

func toDBApproveBatchParams(arg payouts.ApproveBatch) db.ApproveBatchPayoutParams {
	return db.ApproveBatchPayoutParams{
		ID:         arg.BatchID,
		ApprovedBy: nullUUID(arg.ApprovedBy),
	}
}

func toDBRejectBatchParams(arg payouts.RejectBatch) db.RejectBatchPayoutParams {
	return db.RejectBatchPayoutParams{
		ID:            arg.BatchID,
		FailureReason: text(arg.Reason),
	}
}

func toDBSetBatchStatusParams(arg payouts.SetBatchStatus) db.SetBatchPayoutStatusParams {
	return db.SetBatchPayoutStatusParams{
		ID:            arg.BatchID,
		Status:        db.PayoutStatus(arg.Status),
		FailureReason: text(arg.FailureReason),
	}
}

func toDBSetBatchSubmittedParams(arg payouts.SetBatchSubmitted) db.SetBatchPayoutSubmittedParams {
	return db.SetBatchPayoutSubmittedParams{
		ID:              arg.BatchID,
		ProviderBatchID: text(arg.ProviderBatchID),
	}
}

func toDBSetPayoutProviderItemParams(arg payouts.SetPayoutProviderItem) db.SetPayoutProviderItemIdParams {
	return db.SetPayoutProviderItemIdParams{
		ID:                   arg.PayoutID,
		ProviderPayoutItemID: text(arg.ProviderPayoutItemID),
	}
}

func toDBSetPayoutResultParams(arg payouts.SetPayoutResult) db.SetPayoutResultByIdParams {
	return db.SetPayoutResultByIdParams{
		ID:                   arg.PayoutID,
		ProviderPayoutItemID: text(arg.ProviderPayoutItemID),
		Status:               db.PayoutStatus(arg.Status),
		FailureReason:        text(arg.FailureReason),
		ProviderFeeCents:     arg.ProviderFeeCents,
	}
}

func toDBSetPayoutStatusByItemParams(arg payouts.SetPayoutStatusByItem) db.SetPayoutStatusByProviderItemIdParams {
	return db.SetPayoutStatusByProviderItemIdParams{
		ProviderPayoutItemID: text(arg.ProviderPayoutItemID),
		Status:               db.PayoutStatus(arg.Status),
		FailureReason:        text(arg.FailureReason),
		ProviderFeeCents:     arg.ProviderFeeCents,
	}
}

func fromDBDueFund(dbFund db.GetFundsDueForPayoutRow) payouts.DueFund {
	return payouts.DueFund{
		ID:          dbFund.ID,
		Name:        dbFund.Name,
		Frequency:   string(dbFund.PayoutFrequency),
		NextPayment: dbFund.NextPayment.Time,
	}
}
