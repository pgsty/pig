package cmd

import (
	"os"
	"pig/cli/repo"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	repoListDistro string
	repoListArch   string
)

// repoCmd represents the top-level `repo` command
var repoCmd = &cobra.Command{
	Use:     "repo",
	Short:   "Manage OS Software Repositories",
	Aliases: []string{"r"},
	Long: `Description:
    pig repo list
    pig repo set
    pig repo add
    pig repo rm
    pig repo cache
`,
}

// Update the command implementations:
var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available repositories",
	Run: func(cmd *cobra.Command, args []string) {
		switch {
		case repoListArch == "":
			repo.ListRepo(repoListDistro, repoListArch)
		case repoListArch == "amd" || repoListArch == "amd64" || repoListArch == "x86_64":
			repo.ListRepo(repoListDistro, "x86_64")
		case repoListArch == "arm" || repoListArch == "arm64" || repoListArch == "aarch64":
			repo.ListRepo(repoListDistro, "aarch64")
		default:
			repo.ListRepo(repoListDistro, repoListArch)
		}
	},
}

var repoAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add pigsty repository to OS",
	Run: func(cmd *cobra.Command, args []string) {
		err := repo.AddPigstyRepo()
		if err != nil {
			logrus.Error(err)
			os.Exit(1)
		}
	},
}

var repoSetCmd = &cobra.Command{
	Use:   "add [name] [url]",
	Short: "Add repository ot OS",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		return
	},
}

func init() {
	repoListCmd.Flags().StringVarP(&repoListDistro, "distro", "d", "", "list by distribution code")
	repoListCmd.Flags().StringVarP(&repoListArch, "arch", "a", "", "list by architecture amd|arm")

	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoAddCmd)
	rootCmd.AddCommand(repoCmd)
}
