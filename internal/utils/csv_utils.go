package utils

import (
	"encoding/csv"
	"io"
)

func WriteCSV(w io.Writer, data [][]string) error {
	writer := csv.NewWriter(w)

	for _, row := range data {
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}