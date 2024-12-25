package donations

import (
	"boardfund/service/finance"
	"fmt"
	"github.com/spf13/cobra"
)

func DonationsAuditCmd(financeSvc *finance.FinanceService) *cobra.Command {
	return &cobra.Command{
		Use: "donations",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("donations audit\n")
			return nil
		},
	}
}
