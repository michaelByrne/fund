package audit

import "github.com/spf13/cobra"

func AuditCmd() *cobra.Command {
	return &cobra.Command{
		Use: "audit",
	}
}
