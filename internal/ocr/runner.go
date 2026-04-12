package ocr

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ConvertJP2ToPNG converts a JP2 file to PNG using ImageMagick's convert command.
// Output path is returned. If outPath is empty, it is derived from inPath.
func ConvertJP2ToPNG(ctx context.Context, inPath, outPath string) (string, error) {
	if outPath == "" {
		ext := filepath.Ext(inPath)
		outPath = inPath[:len(inPath)-len(ext)] + ".png"
	}

	cmd := exec.CommandContext(ctx, "convert",
		"-density", "300",
		"-depth", "8",
		inPath,
		outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("convert JP2→PNG: %w: %s", err, stderr.String())
	}
	return outPath, nil
}

// TSVRow is one row from tesseract's TSV output.
type TSVRow struct {
	Level      int
	PageNum    int
	BlockNum   int
	ParNum     int
	LineNum    int
	WordNum    int
	Left       int
	Top        int
	Width      int
	Height     int
	Confidence float64
	Text       string
}

// RunTesseract runs tesseract on imgPath and returns the plain text and average confidence.
// lang defaults to "eng" if empty.
func RunTesseract(ctx context.Context, imgPath, lang string) (text string, confidence float64, err error) {
	if lang == "" {
		lang = "eng"
	}

	// First pass: TSV output for confidence scores.
	tsvOut, err := runTesseractOutput(ctx, imgPath, lang, "tsv")
	if err != nil {
		return "", 0, err
	}
	confidence = parseTSVConfidence(tsvOut)

	// Second pass: plain text.
	txtOut, err := runTesseractOutput(ctx, imgPath, lang, "txt")
	if err != nil {
		return "", 0, err
	}
	text = strings.TrimSpace(txtOut)

	return text, confidence, nil
}

func runTesseractOutput(ctx context.Context, imgPath, lang, outputType string) (string, error) {
	cmd := exec.CommandContext(ctx, "tesseract",
		imgPath, "stdout",
		"-l", lang,
		"--oem", "1", // LSTM engine
		"--psm", "1", // automatic page segmentation with OSD
		outputType,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// parseTSVConfidence computes average word confidence from tesseract TSV output.
// Rows with confidence -1 (non-word) are excluded.
func parseTSVConfidence(tsv string) float64 {
	lines := strings.Split(tsv, "\n")
	var total float64
	var count int
	for _, line := range lines[1:] { // skip header
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}
		conf, err := strconv.ParseFloat(fields[10], 64)
		if err != nil || conf < 0 {
			continue
		}
		total += conf
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// CheckDependencies verifies that tesseract and imagemagick convert are available.
func CheckDependencies() error {
	for _, tool := range []string{"tesseract", "convert"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%q not found in PATH: install tesseract-ocr and imagemagick", tool)
		}
	}
	return nil
}
