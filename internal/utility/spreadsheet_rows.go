package utility

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"
)

// CountSpreadsheetRows estimates how many logical rows a table document contains.
// XLSX sums all worksheet rows so task pagination covers every sheet.
func CountSpreadsheetRows(name string, binary []byte) int {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx":
		if rows, err := CountXLSXRows(binary); err == nil {
			return rows
		}
	case ".csv", ".tsv", ".txt":
		return CountDelimitedRows(name, binary)
	}
	return 0
}

func CountDelimitedRows(name string, binary []byte) int {
	reader := csv.NewReader(bytes.NewReader(binary))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	if strings.EqualFold(filepath.Ext(name), ".tsv") {
		reader.Comma = '\t'
	}
	rows := 0
	for {
		_, err := reader.Read()
		if err == nil {
			rows++
			continue
		}
		if err == io.EOF {
			break
		}
		rows += bytes.Count(binary, []byte{'\n'})
		if len(binary) > 0 && binary[len(binary)-1] != '\n' {
			rows++
		}
		break
	}
	return rows
}

func CountXLSXRows(binary []byte) (int, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(binary), int64(len(binary)))
	if err != nil {
		return 0, err
	}
	totalRows := 0
	for _, file := range zipReader.File {
		if !strings.HasPrefix(file.Name, "xl/worksheets/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rows, err := countWorksheetRows(file)
		if err != nil {
			return 0, err
		}
		totalRows += rows
	}
	return totalRows, nil
}

func countWorksheetRows(file *zip.File) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	decoder := xml.NewDecoder(reader)
	rows := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		start, ok := token.(xml.StartElement)
		if ok && start.Name.Local == "row" {
			rows++
		}
	}
	return rows, nil
}
