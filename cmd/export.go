package cmd

import (
	"fmt"

	"github.com/drewstinnett/acs/internal/web"
	"github.com/spf13/cobra"
)

var exportOutDir string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the archive as a static website",
	Long: `Export generates a static HTML site from the local database.

After running export, run pagefind to build the search index:
  npx pagefind --site <out-dir>

Then open <out-dir>/index.html in a browser or deploy to GitHub Pages.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if exportOutDir == "" {
			return fmt.Errorf("--out-dir is required")
		}

		db, err := openDB()
		if err != nil {
			return err
		}
		defer db.Close()

		exporter := web.NewExporter(db, exportOutDir, log)
		if err := exporter.Export(); err != nil {
			return fmt.Errorf("export: %w", err)
		}

		fmt.Printf("\nStatic site written to: %s\n", exportOutDir)
		fmt.Println("Next step: run pagefind to build the search index:")
		fmt.Printf("  npx pagefind --site %s\n", exportOutDir)
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportOutDir, "out-dir", "", "output directory for the static site (required)")
	rootCmd.AddCommand(exportCmd)
}
