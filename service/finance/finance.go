package finance

import (
	"boardfund/service/donations"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
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
	Status            string
	AmountCents       int32
}

type ReportInfo struct {
	FundID uuid.UUID
	Date   time.Time
	Type   string
}

type donationStore interface {
	GetRecurringDonationsForFund(ctx context.Context, arg donations.GetRecurringDonationsForFundRequest) ([]donations.Donation, error)
	GetPaymentsForDonation(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
	GetActiveFunds(ctx context.Context) ([]donations.Fund, error)
	SetDonationToInactive(ctx context.Context, arg donations.DeactivateDonation) (*donations.Donation, error)
	GetDonationPaymentsByDonationID(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
}

type paymentsProvider interface {
	GetProviderDonationSubscriptionStatus(ctx context.Context, providerSubscriptionID string) (string, error)
	GetTransactionsForDonationSubscription(ctx context.Context, subscriptionID string) ([]ProviderTransaction, error)
	GetTransaction(ctx context.Context, id string, start, end time.Time) (*ProviderTransaction, error)
}

type documentManager interface {
	Upload(ctx context.Context, body io.Reader, name, contentType string) error
	ListAvailableReports(ctx context.Context, prefix string, fundID uuid.UUID) ([]ReportInfo, error)
}

type FinanceService struct {
	donationStore    donationStore
	paymentsProvider paymentsProvider
	uploader         documentManager

	logger *slog.Logger
}

func NewFinanceService(donationStore donationStore, paymentsProvider paymentsProvider, uploader documentManager, logger *slog.Logger) *FinanceService {
	return &FinanceService{
		donationStore:    donationStore,
		paymentsProvider: paymentsProvider,
		uploader:         uploader,
		logger:           logger,
	}
}

func (s FinanceService) GetAvailableReportDates(ctx context.Context, reportType string, fundID uuid.UUID) ([]time.Time, error) {
	reports, err := s.uploader.ListAvailableReports(ctx, reportType, fundID)
	if err != nil {
		s.logger.Error("failed to get available reports", slog.String("error", err.Error()))

		return nil, err
	}

	dates := make([]time.Time, 0)
	for _, report := range reports {
		dates = append(dates, report.Date)
	}

	return dates, nil
}

func (s FinanceService) RunDonationReconciliation(ctx context.Context) error {
	funds, err := s.donationStore.GetActiveFunds(ctx)
	if err != nil {
		s.logger.Error("failed to get active funds", slog.String("error", err.Error()))

		return err
	}

	for _, fund := range funds {
		errInner := s.reconcileDonationsForFund(ctx, fund.ID)
		if errInner != nil {
			return errInner
		}
	}

	return nil
}

func (s FinanceService) reconcileDonationsForFund(ctx context.Context, fundID uuid.UUID) error {
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

	fmt.Printf(" to write %s", bytesBuffer.String())
	fileName := "fund_" + fundID.String() + "_date_" + time.Now().Format("01-02-2006") + "_payments_report.csv"
	err = s.uploader.Upload(ctx, bytesBuffer, fileName, "text/csv")
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
	})
}
