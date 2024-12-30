package finance

import (
	"boardfund/service/donations"
	"bytes"
	"context"
	"encoding/csv"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

type AuditDonation struct {
	Active                 bool
	ProviderSubscriptionID string
	FirstName              string
	LastName               string
}

type ProviderTransaction struct {
	ProviderPaymentID string
	Date              time.Time
	Status            string
	AmountCents       int32
	FeeCents          int32
}

type ReportInfo struct {
	FundID uuid.UUID
	Date   time.Time
	Type   string
}

type Audit struct {
	FundID   uuid.UUID
	FundName string
	Date     time.Time
	Type     string
	Payments []AuditPayment
}

type AuditPayment struct {
	DonationID             uuid.UUID
	PaymentID              uuid.UUID
	ProviderPaymentID      string
	AmountCents            int32
	FeeAmountCents         int32
	TransactionStatus      string
	TransactionAmountCents int32
	Created                time.Time
	ProviderCreated        time.Time
}

type GetAuditRequest struct {
	FundID uuid.UUID
	Type   string
	Date   time.Time
}

type donationStore interface {
	GetRecurringDonationsForFund(ctx context.Context, arg donations.GetRecurringDonationsForFundRequest) ([]donations.Donation, error)
	GetPaymentsForDonation(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
	GetActiveFunds(ctx context.Context, freq string) ([]donations.Fund, error)
	SetDonationToInactive(ctx context.Context, arg donations.DeactivateDonation) (*donations.Donation, error)
	GetDonationPaymentsByDonationID(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
	GetFundByID(ctx context.Context, uuid uuid.UUID) (*donations.Fund, error)
	GetOneTimeDonationsForFund(ctx context.Context, arg donations.GetOneTimeDonationsForFundRequest) ([]donations.Donation, error)
	UpdatePaymentPaypalFee(ctx context.Context, arg donations.UpdatePaymentPaypalFee) (*donations.DonationPayment, error)
}

type paymentsProvider interface {
	GetProviderDonationSubscriptionStatus(ctx context.Context, providerSubscriptionID string) (string, error)
	GetTransactionsForDonationSubscription(ctx context.Context, subscriptionID string) ([]ProviderTransaction, error)
	GetTransaction(ctx context.Context, id string, start, end time.Time) (*ProviderTransaction, error)
}

type documentManager interface {
	Upload(ctx context.Context, body io.Reader, name, contentType string) error
	ListAvailableReports(ctx context.Context, prefix string, fundID uuid.UUID) ([]ReportInfo, error)
	GetReport(ctx context.Context, reportType string, fundID uuid.UUID, date time.Time) (io.Reader, error)
}

type FinanceService struct {
	donationStore    donationStore
	paymentsProvider paymentsProvider
	documentManager  documentManager

	reportPrefixes []string

	logger *slog.Logger
}

func NewFinanceService(donationStore donationStore, paymentsProvider paymentsProvider, documentManager documentManager, reportPrefixes []string, logger *slog.Logger) *FinanceService {
	return &FinanceService{
		donationStore:    donationStore,
		paymentsProvider: paymentsProvider,
		documentManager:  documentManager,
		reportPrefixes:   reportPrefixes,
		logger:           logger,
	}
}

func (s FinanceService) GetAudit(ctx context.Context, req GetAuditRequest) (*Audit, error) {
	reportReader, err := s.documentManager.GetReport(ctx, req.Type, req.FundID, req.Date)
	if err != nil {
		s.logger.Error("failed to get report", slog.String("error", err.Error()))

		return nil, err
	}

	fund, err := s.donationStore.GetFundByID(ctx, req.FundID)
	if err != nil {
		s.logger.Error("failed to get fund by ID", slog.String("error", err.Error()))

		return nil, err
	}

	csvReader := csv.NewReader(reportReader)
	records, err := csvReader.ReadAll()
	if err != nil {
		s.logger.Error("failed to read CSV records", slog.String("error", err.Error()))

		return nil, err
	}

	payments := make([]AuditPayment, 0, len(records))
	for _, record := range records {
		amountCents, errInner := strconv.Atoi(record[5])
		if errInner != nil {
			s.logger.Error("failed to parse amount cents", slog.String("error", errInner.Error()))

			return nil, errInner
		}

		transactionAmountCentsStr := record[8]
		if transactionAmountCentsStr == "" {
			transactionAmountCentsStr = "0"
		}

		transactionAmountCents, errInner := strconv.Atoi(transactionAmountCentsStr)
		if errInner != nil {
			s.logger.Error("failed to parse transaction amount cents", slog.String("error", errInner.Error()))

			return nil, errInner
		}

		donationUUID, errInner := uuid.Parse(record[2])
		if errInner != nil {
			s.logger.Error("failed to parse donation UUID", slog.String("error", err.Error()))

			return nil, errInner
		}

		paymentUUID, errInner := uuid.Parse(record[1])
		if errInner != nil {
			s.logger.Error("failed to parse payment UUID", slog.String("error", err.Error()))

			return nil, errInner
		}

		created := record[4]
		createdTime, errInner := time.Parse(time.RFC3339, created)
		if errInner != nil {
			s.logger.Error("failed to parse created time", slog.String("error", err.Error()))

			return nil, errInner
		}

		providerCreatedStr := record[9]
		if providerCreatedStr == "" {
			providerCreatedStr = time.Time{}.Format(time.RFC3339)
		}

		providerCreated, errInner := time.Parse(time.RFC3339, providerCreatedStr)
		if errInner != nil {
			s.logger.Error("failed to parse provider created time", slog.String("error", err.Error()))

			return nil, errInner
		}

		feeStr := record[10]
		if feeStr == "" {
			feeStr = "0"
		}

		feeAmountCents, errInner := strconv.Atoi(feeStr)
		if errInner != nil {
			s.logger.Error("failed to parse fee amount cents", slog.String("error", errInner.Error()))

			return nil, errInner
		}

		payment := AuditPayment{
			DonationID:             donationUUID,
			PaymentID:              paymentUUID,
			ProviderPaymentID:      record[3],
			AmountCents:            int32(amountCents),
			TransactionStatus:      record[7],
			TransactionAmountCents: int32(transactionAmountCents),
			Created:                createdTime,
			ProviderCreated:        providerCreated,
			FeeAmountCents:         int32(feeAmountCents),
		}

		payments = append(payments, payment)
	}

	audit := &Audit{
		FundID:   req.FundID,
		FundName: fund.Name,
		Date:     req.Date,
		Type:     req.Type,
		Payments: payments,
	}

	return audit, nil
}

func (s FinanceService) GetAvailableAudits(ctx context.Context, fundID uuid.UUID) ([]ReportInfo, error) {
	var allReports []ReportInfo
	for _, prefix := range s.reportPrefixes {
		reports, err := s.documentManager.ListAvailableReports(ctx, prefix, fundID)
		if err != nil {
			s.logger.Error("failed to get available reports", slog.String("error", err.Error()))

			return nil, err
		}

		allReports = append(allReports, reports...)
	}

	return allReports, nil
}

func (s FinanceService) RunOneTimeDonationReconciliation(ctx context.Context) error {
	funds, err := s.donationStore.GetActiveFunds(ctx, "once")
	if err != nil {
		s.logger.Error("failed to get active funds", slog.String("error", err.Error()))

		return err
	}

	for _, fund := range funds {
		errInner := s.reconcileOneTimeDonationsForFund(ctx, fund.ID)
		if errInner != nil {
			return errInner
		}
	}

	return nil
}

func (s FinanceService) RunRecurringDonationReconciliation(ctx context.Context) error {
	funds, err := s.donationStore.GetActiveFunds(ctx, "monthly")
	if err != nil {
		s.logger.Error("failed to get active funds", slog.String("error", err.Error()))

		return err
	}

	for _, fund := range funds {
		errInner := s.reconcileRecurringDonationsForFund(ctx, fund.ID)
		if errInner != nil {
			return errInner
		}
	}

	return nil
}

func (s FinanceService) reconcileRecurringDonationsForFund(ctx context.Context, fundID uuid.UUID) error {
	bytesBuffer := bytes.NewBuffer([]byte{})
	csvWriter := csv.NewWriter(bytesBuffer)

	logger := s.logger.With(slog.String("fund_id", fundID.String()), slog.String("date", time.Now().Format("01-02-2006")))

	req := donations.GetRecurringDonationsForFundRequest{
		FundID: fundID,
		Active: true,
	}

	recurringDonations, err := s.donationStore.GetRecurringDonationsForFund(ctx, req)
	if err != nil {
		logger.Error("failed to get recurring donations for fund", slog.String("error", err.Error()))

		return err
	}

	for _, donation := range recurringDonations {
		status, errInner := s.paymentsProvider.GetProviderDonationSubscriptionStatus(ctx, donation.ProviderSubscriptionID)
		if errInner != nil {
			logger.Error("failed to get donation status from provider", slog.String("error", errInner.Error()))
		}

		if !(strings.ToUpper(status) == "ACTIVE") {
			logger.Info("donation is inactive at provider", slog.String("donation_id", donation.ID.String()))

			_, errInner = s.donationStore.SetDonationToInactive(ctx, donations.DeactivateDonation{
				ID:     donation.ID,
				Reason: status,
			})
			if errInner != nil {
				logger.Error("failed to set donation to inactive", slog.String("error", errInner.Error()))

				return errInner
			}
		}

		payments, errInner := s.donationStore.GetDonationPaymentsByDonationID(ctx, donation.ID)
		if errInner != nil {
			logger.Error("failed to get donation payments", slog.String("error", errInner.Error()))

			return errInner
		}

		for _, payment := range payments {
			transaction, errTrans := s.paymentsProvider.GetTransaction(ctx, payment.ProviderPaymentID, payment.Created.AddDate(0, 0, -1), time.Now())
			if errTrans != nil {
				logger.Error("failed to get transaction from provider", slog.String("error", errTrans.Error()))
				errCSV := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
				if errCSV != nil {
					logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCSV.Error()))
				}

				continue
			}

			if transaction == nil {
				logger.Info("transaction not found", slog.String("payment_id", payment.ID.String()))
				errCSV := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
				if errCSV != nil {
					logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCSV.Error()))
				}

				continue
			}

			if !(strings.ToUpper(transaction.Status) == "COMPLETED") {
				logger.Info("payment is incomplete at provider", slog.String("payment_id", payment.ID.String()))
				errCsv := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
				if errCsv != nil {
					logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCsv.Error()))
				}

				continue
			}

			if transaction.AmountCents != payment.AmountCents {
				logger.Info("payment amount does not match provider", slog.String("payment_id", payment.ID.String()), slog.Int("expected", int(payment.AmountCents)), slog.Int("actual", int(transaction.AmountCents)))
			}

			errCSV := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
			if errCSV != nil {
				logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCSV.Error()))
			}
		}
	}

	csvWriter.Flush()
	err = csvWriter.Error()
	if err != nil {
		logger.Error("failed to flush CSV writer for donations payment report", slog.String("error", err.Error()))

		return err
	}

	fileName := "fund_" + fundID.String() + "_date_" + time.Now().Format("01-02-2006") + "_payments_report.csv"
	err = s.documentManager.Upload(ctx, bytesBuffer, fileName, "text/csv")
	if err != nil {
		logger.Error("failed to upload CSV file for donations payment report", slog.String("error", err.Error()))
	}

	return nil
}

func (s FinanceService) reconcileOneTimeDonationsForFund(ctx context.Context, fundID uuid.UUID) error {
	bytesBuffer := bytes.NewBuffer([]byte{})
	csvWriter := csv.NewWriter(bytesBuffer)

	logger := s.logger.With(slog.String("fund_id", fundID.String()), slog.String("date", time.Now().Format("01-02-2006")))

	req := donations.GetOneTimeDonationsForFundRequest{
		FundID: fundID,
		Active: true,
	}

	oneTimeDonations, err := s.donationStore.GetOneTimeDonationsForFund(ctx, req)
	if err != nil {
		logger.Error("failed to get one-time donations for fund", slog.String("error", err.Error()))

		return err
	}

	for _, donation := range oneTimeDonations {
		payments, errInner := s.donationStore.GetDonationPaymentsByDonationID(ctx, donation.ID)
		if errInner != nil {
			logger.Error("failed to get donation payments", slog.String("error", errInner.Error()))

			return errInner
		}

		for _, payment := range payments {
			transaction, errTrans := s.paymentsProvider.GetTransaction(ctx, payment.ProviderPaymentID, payment.Created.AddDate(0, 0, -1), time.Now())
			if errTrans != nil {
				logger.Error("failed to get transaction from provider", slog.String("error", errTrans.Error()))
				errCSV := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
				if errCSV != nil {
					logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCSV.Error()))
				}

				continue
			}

			if transaction == nil {
				logger.Info("transaction not found", slog.String("payment_id", payment.ID.String()))
				errCSV := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
				if errCSV != nil {
					logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCSV.Error()))
				}

				continue
			}

			_, err = s.donationStore.UpdatePaymentPaypalFee(ctx, donations.UpdatePaymentPaypalFee{
				ID:               payment.ID,
				ProviderFeeCents: transaction.FeeCents,
			})
			if err != nil {
				logger.Error("failed to update payment fee", slog.String("error", err.Error()))

				return err
			}

			if !(strings.ToUpper(transaction.Status) == "COMPLETED") {
				logger.Info("payment is incomplete at provider", slog.String("payment_id", payment.ID.String()))
				errCsv := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
				if errCsv != nil {
					logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCsv.Error()))
				}

				continue
			}

			if transaction.AmountCents != payment.AmountCents {
				logger.Info("payment amount does not match provider", slog.String("payment_id", payment.ID.String()), slog.Int("expected", int(payment.AmountCents)), slog.Int("actual", int(transaction.AmountCents)))
			}

			errCSV := writeCSVPaymentRow(csvWriter, fundID, payment, transaction)
			if errCSV != nil {
				logger.Error("failed to write CSV row in donations payments report", slog.String("error", errCSV.Error()))
			}
		}
	}

	csvWriter.Flush()
	err = csvWriter.Error()
	if err != nil {
		logger.Error("failed to flush CSV writer for donations payment report", slog.String("error", err.Error()))

		return err
	}

	fileName := "fund_" + fundID.String() + "_date_" + time.Now().Format("01-02-2006") + "_payments_report.csv"
	err = s.documentManager.Upload(ctx, bytesBuffer, fileName, "text/csv")
	if err != nil {
		logger.Error("failed to upload CSV file for donations payment report", slog.String("error", err.Error()))
	}

	return nil
}

func writeCSVPaymentRow(writer *csv.Writer, fundID uuid.UUID, payment donations.DonationPayment, transaction *ProviderTransaction) error {
	if transaction == nil {
		return writer.Write([]string{
			fundID.String(),
			payment.ID.String(),
			payment.DonationID.String(),
			payment.ProviderPaymentID,
			payment.Created.Format(time.RFC3339),
			strconv.Itoa(int(payment.AmountCents)),
			"",
			"",
			"",
			"",
			"",
		})
	}

	return writer.Write([]string{
		fundID.String(),
		payment.ID.String(),
		payment.DonationID.String(),
		payment.ProviderPaymentID,
		payment.Created.Format(time.RFC3339),
		strconv.Itoa(int(payment.AmountCents)),
		transaction.ProviderPaymentID,
		transaction.Status,
		strconv.Itoa(int(transaction.AmountCents)),
		transaction.Date.Format(time.RFC3339),
		strconv.Itoa(int(payment.ProviderFeeCents)),
	})
}
