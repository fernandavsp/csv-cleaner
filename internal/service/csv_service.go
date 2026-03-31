package service

import (
	"bytes"
	"encoding/csv"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type CleanResult struct {
	Data             [][]string
	RemovedRows      int
	TrimmedCells     int
	EmptyRowsRemoved int
	DateFormatted    int
	DateInvalid      int
	DateChecked      int
}

const (
	DateFormatISO      = "iso"
	DateFormatBR       = "br"
	DateFormatUS       = "us"
	DateFormatNoChange = "none"
)

func ParseCSV(file io.Reader) ([][]string, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	// Remove BOM UTF-8 gerado pelo Excel
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		content = content[3:]
	}

	firstLine := strings.SplitN(string(content), "\n", 2)[0]
	delimiter := detectDelimiter(firstLine)

	reader := csv.NewReader(bytes.NewReader(content))
	reader.TrimLeadingSpace = true
	reader.Comma = delimiter
	reader.LazyQuotes = true

	return reader.ReadAll()
}

// detectDelimiter analisa a primeira linha e retorna o delimitador mais provável.
func detectDelimiter(line string) rune {
	candidates := []rune{',', ';', '\t', '|'}
	best := ','
	bestCount := 0
	for _, r := range candidates {
		if c := strings.Count(line, string(r)); c > bestCount {
			bestCount = c
			best = r
		}
	}
	return best
}

func CleanData(data [][]string, dateFormat string) CleanResult {
	before := len(data)

	var trimmedCount int
	var emptyRemoved int
	var dateFormatted int
	var dateInvalid int
	var dateChecked int

	data, trimmedCount = TrimSpaces(data)
	data, emptyRemoved = RemoveEmptyRows(data)
	data, dateFormatted, dateInvalid, dateChecked = NormalizeDates(data, dateFormat)
	data = RemoveDuplicates(data)

	after := len(data)

	return CleanResult{
		Data:             data,
		RemovedRows:      before - after,
		TrimmedCells:     trimmedCount,
		EmptyRowsRemoved: emptyRemoved,
		DateFormatted:    dateFormatted,
		DateInvalid:      dateInvalid,
		DateChecked:      dateChecked,
	}
}

func NormalizeDates(data [][]string, dateFormat string) ([][]string, int, int, int) {
	if len(data) == 0 {
		return data, 0, 0, 0
	}

	layout, ok := outputLayout(dateFormat)
	dateColumns := detectDateColumns(data[0])
	if len(dateColumns) == 0 {
		return data, 0, 0, 0
	}

	count := 0
	invalid := 0
	checked := 0

	for i := 1; i < len(data); i++ {
		for _, j := range dateColumns {
			if j >= len(data[i]) {
				continue
			}

			value := strings.TrimSpace(data[i][j])
			if value == "" {
				continue
			}

			checked++
			parsed, parsedOK := parseKnownDate(value)
			if !parsedOK {
				if looksLikeDate(value) {
					invalid++
				}
				continue
			}

			if !ok {
				continue
			}

			formatted := parsed.Format(layout)
			if formatted != data[i][j] {
				data[i][j] = formatted
				count++
			}
		}
	}

	return data, count, invalid, checked
}

func detectDateColumns(header []string) []int {
	var cols []int
	for i, col := range header {
		normalized := strings.ToLower(strings.TrimSpace(col))
		if normalized == "data" || normalized == "date" || normalized == "dt" || strings.Contains(normalized, "data") || strings.Contains(normalized, "date") {
			cols = append(cols, i)
		}
	}
	return cols
}

var dateLikeRegex = regexp.MustCompile(`^\d{1,4}[-/]\d{1,2}[-/]\d{1,4}(?:[ tT].*)?$`)

func looksLikeDate(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	return dateLikeRegex.MatchString(v)
}

func outputLayout(dateFormat string) (string, bool) {
	switch strings.ToLower(dateFormat) {
	case DateFormatISO:
		return "2006-01-02", true
	case DateFormatBR:
		return "02/01/2006", true
	case DateFormatUS:
		return "01/02/2006", true
	default:
		return "", false
	}
}

func parseKnownDate(value string) (time.Time, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return time.Time{}, false
	}

	if t, ok := parseNumericDate(v); ok {
		return t, true
	}

	layouts := []string{
		"2006-01-02",
		"02-01-2006",
		"01-02-2006",
		"2006/01/02",
		"02/01/2006",
		"01/02/2006",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, v)
		if err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}

func parseNumericDate(value string) (time.Time, bool) {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		parts = strings.Split(value, "-")
	}
	if len(parts) != 3 {
		return time.Time{}, false
	}

	a, errA := strconv.Atoi(parts[0])
	b, errB := strconv.Atoi(parts[1])
	c, errC := strconv.Atoi(parts[2])
	if errA != nil || errB != nil || errC != nil {
		return time.Time{}, false
	}

	if len(parts[0]) == 4 {
		return safeDate(a, b, c)
	}

	if a > 12 {
		return safeDate(c, b, a)
	}

	if b > 12 {
		return safeDate(c, a, b)
	}

	// Ambíguo (ex: 01/02/2024): prioriza padrão BR (dd/mm/yyyy)
	return safeDate(c, b, a)
}

func safeDate(year, month, day int) (time.Time, bool) {
	if year < 1000 || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}

	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return time.Time{}, false
	}

	return t, true
}

func TrimSpaces(data [][]string) ([][]string, int) {
	count := 0

	for i := range data {
		for j := range data[i] {
			original := data[i][j]
			trimmed := strings.TrimSpace(original)

			if original != trimmed {
				count++
			}

			data[i][j] = trimmed
		}
	}

	return data, count
}

func RemoveDuplicates(data [][]string) [][]string {
	seen := make(map[string]bool)
	var result [][]string

	for _, row := range data {
		key := strings.Join(row, "|")

		if !seen[key] {
			seen[key] = true
			result = append(result, row)
		}
	}

	return result
}

func RemoveEmptyRows(data [][]string) ([][]string, int) {
	var result [][]string
	removed := 0

	for _, row := range data {
		isEmpty := true

		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				isEmpty = false
				break
			}
		}

		if isEmpty {
			removed++
			continue
		}

		result = append(result, row)
	}

	return result, removed
}
