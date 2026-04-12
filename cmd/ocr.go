package cmd

import (
	"fmt"

	"github.com/drewstinnett/acs/internal/ocr"
	"github.com/spf13/cobra"
)

var (
	ocrItem    string
	ocrWorkers int
	ocrLang    string
	ocrKeepPNG bool
	ocrRerun   bool
)

var ocrCmd = &cobra.Command{
	Use:   "ocr",
	Short: "Run OCR on downloaded microfilm images",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ocr.CheckDependencies(); err != nil {
			return fmt.Errorf("dependency check: %w", err)
		}

		db, err := openDB()
		if err != nil {
			return err
		}
		defer db.Close()

		opts := ocr.Options{
			Workers: ocrWorkers,
			Lang:    ocrLang,
			KeepPNG: ocrKeepPNG,
			Rerun:   ocrRerun,
			ItemID:  ocrItem,
			DataDir: dataDir,
		}

		proc := ocr.NewProcessor(db, opts, log)
		return proc.Run(cmd.Context())
	},
}

func init() {
	ocrCmd.Flags().StringVar(&ocrItem, "item", "", "restrict OCR to a specific IA item")
	ocrCmd.Flags().IntVar(&ocrWorkers, "workers", 2, "parallel OCR workers")
	ocrCmd.Flags().StringVar(&ocrLang, "lang", "eng", "Tesseract language code")
	ocrCmd.Flags().BoolVar(&ocrKeepPNG, "keep-png", false, "keep converted PNG files after OCR")
	ocrCmd.Flags().BoolVar(&ocrRerun, "rerun", false, "re-run OCR even on already-processed pages")
}
