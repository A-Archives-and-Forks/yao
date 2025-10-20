package seed

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
	"github.com/yaoapp/gou/application"
	"github.com/yaoapp/gou/fs"
	"github.com/yaoapp/gou/model"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/kun/maps"
)

// Import imports seed data from file into model
func Import(filename string, modelName string, options ImportOption) (*ImportResult, error) {
	// Get model
	mod := model.Select(modelName)

	// Initialize result
	result := &ImportResult{
		Total:   0,
		Success: 0,
		Failure: 0,
		Ignore:  0,
		Errors:  []ImportError{},
	}

	// Determine file type and import
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".csv":
		return result, importDataFromCSV(filename, mod, options, result)
	case ".xlsx", ".xls":
		return result, importDataFromXLSX(filename, mod, options, result)
	case ".json":
		return result, importDataFromJSON(filename, mod, options, result)
	case ".yao", ".jsonc":
		return result, importDataFromYao(filename, mod, options, result)
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}
}

// importDataFromCSV import data from CSV file
func importDataFromCSV(filename string, mod *model.Model, options ImportOption, result *ImportResult) error {
	// Read file from seed filesystem
	seedFS := fs.MustGet("seed")
	data, err := seedFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read CSV file: %v", err)
	}

	// Parse CSV
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %v", err)
	}

	// Prepare handler
	handler := createImportHandler(mod, header, options, result)

	// Read data in chunks
	chunk := [][]interface{}{}
	lineNum := 1 // Start from 1 (header is line 0)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, ImportError{
				Row:     lineNum,
				Message: err.Error(),
				Code:    500,
			})
			result.Failure++
			result.Total++
			lineNum++
			continue
		}

		// Convert to interface slice
		row := make([]interface{}, len(record))
		for i, v := range record {
			row[i] = v
		}

		chunk = append(chunk, row)
		result.Total++

		// Process chunk when size reached
		if len(chunk) >= options.ChunkSize {
			if err := handler(lineNum-len(chunk)+1, chunk); err != nil {
				log.Error("Import chunk error: %v", err)
			}
			chunk = [][]interface{}{}
		}

		lineNum++
	}

	// Process remaining chunk
	if len(chunk) > 0 {
		if err := handler(lineNum-len(chunk), chunk); err != nil {
			log.Error("Import final chunk error: %v", err)
		}
	}

	return nil
}

// importDataFromXLSX import data from XLSX file
func importDataFromXLSX(filename string, mod *model.Model, options ImportOption, result *ImportResult) error {
	// Read file from seed filesystem
	seedFS := fs.MustGet("seed")
	data, err := seedFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read XLSX file: %v", err)
	}

	// Open Excel file from bytes
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to open XLSX file: %v", err)
	}
	defer file.Close()

	// Get active sheet
	sheetName := file.GetSheetName(file.GetActiveSheetIndex())
	rows, err := file.Rows(sheetName)
	if err != nil {
		return fmt.Errorf("failed to get rows: %v", err)
	}
	defer rows.Close()

	// Read header
	if !rows.Next() {
		return fmt.Errorf("empty XLSX file")
	}
	header, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to read header: %v", err)
	}

	// Prepare handler
	handler := createImportHandler(mod, header, options, result)

	// Read data in chunks
	chunk := [][]interface{}{}
	lineNum := 1 // Header is line 0, data starts from 1

	for rows.Next() {
		record, err := rows.Columns()
		if err != nil {
			result.Errors = append(result.Errors, ImportError{
				Row:     lineNum,
				Message: err.Error(),
				Code:    500,
			})
			result.Failure++
			result.Total++
			lineNum++
			continue
		}

		// Check if row is empty
		isEmpty := true
		for _, v := range record {
			if v != "" {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			lineNum++
			continue
		}

		// Convert to interface slice
		row := make([]interface{}, len(record))
		for i, v := range record {
			row[i] = v
		}

		chunk = append(chunk, row)
		result.Total++

		// Process chunk when size reached
		if len(chunk) >= options.ChunkSize {
			if err := handler(lineNum-len(chunk)+1, chunk); err != nil {
				log.Error("Import chunk error: %v", err)
			}
			chunk = [][]interface{}{}
		}

		lineNum++
	}

	// Process remaining chunk
	if len(chunk) > 0 {
		if err := handler(lineNum-len(chunk), chunk); err != nil {
			log.Error("Import final chunk error: %v", err)
		}
	}

	return nil
}

// importDataFromJSON import data from JSON file
func importDataFromJSON(filename string, mod *model.Model, options ImportOption, result *ImportResult) error {
	// Read file from seed filesystem
	seedFS := fs.MustGet("seed")
	data, err := seedFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %v", err)
	}

	// Parse JSON - expect array of objects
	var records []map[string]interface{}
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	if len(records) == 0 {
		return nil
	}

	// Extract columns from first record
	columns := []string{}
	for key := range records[0] {
		columns = append(columns, key)
	}

	// Convert to rows format
	handler := createJSONImportHandler(mod, columns, options, result)

	// Process records in chunks
	chunk := []map[string]interface{}{}
	for i, record := range records {
		result.Total++
		chunk = append(chunk, record)

		if len(chunk) >= options.ChunkSize {
			if err := handler(i-len(chunk)+1, chunk); err != nil {
				log.Error("Import chunk error: %v", err)
			}
			chunk = []map[string]interface{}{}
		}
	}

	// Process remaining chunk
	if len(chunk) > 0 {
		if err := handler(len(records)-len(chunk), chunk); err != nil {
			log.Error("Import final chunk error: %v", err)
		}
	}

	return nil
}

// importDataFromYao import data from Yao file (JSONC)
func importDataFromYao(filename string, mod *model.Model, options ImportOption, result *ImportResult) error {
	// Read file from seed filesystem
	seedFS := fs.MustGet("seed")
	data, err := seedFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read Yao file: %v", err)
	}

	// Parse using application Parse (handles JSONC)
	var records []map[string]interface{}
	if err := application.Parse(filename, data, &records); err != nil {
		return fmt.Errorf("failed to parse Yao file: %v", err)
	}

	if len(records) == 0 {
		return nil
	}

	// Extract columns from first record
	columns := []string{}
	for key := range records[0] {
		columns = append(columns, key)
	}

	// Convert to rows format
	handler := createJSONImportHandler(mod, columns, options, result)

	// Process records in chunks
	chunk := []map[string]interface{}{}
	for i, record := range records {
		result.Total++
		chunk = append(chunk, record)

		if len(chunk) >= options.ChunkSize {
			if err := handler(i-len(chunk)+1, chunk); err != nil {
				log.Error("Import chunk error: %v", err)
			}
			chunk = []map[string]interface{}{}
		}
	}

	// Process remaining chunk
	if len(chunk) > 0 {
		if err := handler(len(records)-len(chunk), chunk); err != nil {
			log.Error("Import final chunk error: %v", err)
		}
	}

	return nil
}

// createImportHandler creates handler for CSV/XLSX format
func createImportHandler(mod *model.Model, columns []string, options ImportOption, result *ImportResult) ImportHandler {
	return func(line int, data [][]interface{}) error {
		if options.Mode == ImportModeEach {
			// Single record mode - use Create
			return importEach(mod, columns, data, line, options, result)
		}
		// Batch mode - use Insert
		return importBatch(mod, columns, data, line, options, result)
	}
}

// createJSONImportHandler creates handler for JSON/Yao format
func createJSONImportHandler(mod *model.Model, columns []string, options ImportOption, result *ImportResult) func(line int, data []map[string]interface{}) error {
	return func(line int, data []map[string]interface{}) error {
		if options.Mode == ImportModeEach {
			// Single record mode - use Create or Save
			return importEachJSON(mod, data, line, options, result)
		}
		// Batch mode - convert to rows and use Insert
		rows := make([][]interface{}, len(data))
		for i, record := range data {
			row := make([]interface{}, len(columns))
			for j, col := range columns {
				row[j] = record[col]
			}
			rows[i] = row
		}
		return importBatch(mod, columns, rows, line, options, result)
	}
}

// importBatch batch import using Model.Insert
func importBatch(mod *model.Model, columns []string, data [][]interface{}, startLine int, options ImportOption, result *ImportResult) error {
	if len(data) == 0 {
		return nil
	}

	switch options.Duplicate {
	case DuplicateIgnore:
		// Try to insert, ignore errors
		err := mod.Insert(columns, data)
		if err != nil {
			// Log error but don't fail
			log.Warn("Batch insert with ignore strategy: %v", err)
			result.Ignore += len(data)
		} else {
			result.Success += len(data)
		}

	case DuplicateError:
		// Insert and fail on error
		err := mod.Insert(columns, data)
		if err != nil {
			for i := range data {
				result.Errors = append(result.Errors, ImportError{
					Row:     startLine + i,
					Message: err.Error(),
					Code:    500,
					Data:    data[i],
				})
			}
			result.Failure += len(data)
			return err
		}
		result.Success += len(data)

	case DuplicateUpdate, DuplicateAbort:
		// For update/abort, fall back to each mode
		for i, row := range data {
			rowMap := maps.MakeMapStrAny()
			for j, col := range columns {
				if j < len(row) {
					rowMap[col] = row[j]
				}
			}
			if err := handleDuplicate(mod, rowMap, startLine+i, options.Duplicate, result); err != nil {
				if options.Duplicate == DuplicateAbort {
					return err
				}
			}
		}
	}

	return nil
}

// importEach single record import using Model.Create
func importEach(mod *model.Model, columns []string, data [][]interface{}, startLine int, options ImportOption, result *ImportResult) error {
	for i, row := range data {
		// Convert row to map
		rowMap := maps.MakeMapStrAny()
		for j, col := range columns {
			if j < len(row) {
				rowMap[col] = row[j]
			}
		}

		if err := handleDuplicate(mod, rowMap, startLine+i, options.Duplicate, result); err != nil {
			if options.Duplicate == DuplicateAbort {
				return err
			}
		}
	}
	return nil
}

// importEachJSON single record import for JSON format
func importEachJSON(mod *model.Model, data []map[string]interface{}, startLine int, options ImportOption, result *ImportResult) error {
	for i, record := range data {
		rowMap := maps.MapStrAny(record)
		if err := handleDuplicate(mod, rowMap, startLine+i, options.Duplicate, result); err != nil {
			if options.Duplicate == DuplicateAbort {
				return err
			}
		}
	}
	return nil
}

// handleDuplicate handles duplicate strategy for single record
func handleDuplicate(mod *model.Model, row maps.MapStrAny, line int, duplicateMode DuplicateMode, result *ImportResult) error {
	switch duplicateMode {
	case DuplicateIgnore:
		// Try to create, ignore if exists
		_, err := mod.Create(row)
		if err != nil {
			result.Ignore++
			log.Debug("Row %d ignored: %v", line, err)
		} else {
			result.Success++
		}

	case DuplicateUpdate:
		// Use Save (create or update)
		_, err := mod.Save(row)
		if err != nil {
			result.Errors = append(result.Errors, ImportError{
				Row:     line,
				Message: err.Error(),
				Code:    500,
			})
			result.Failure++
		} else {
			result.Success++
		}

	case DuplicateError:
		// Create and fail on error
		_, err := mod.Create(row)
		if err != nil {
			result.Errors = append(result.Errors, ImportError{
				Row:     line,
				Message: err.Error(),
				Code:    500,
			})
			result.Failure++
			return err
		}
		result.Success++

	case DuplicateAbort:
		// Create and abort on error
		_, err := mod.Create(row)
		if err != nil {
			result.Errors = append(result.Errors, ImportError{
				Row:     line,
				Message: err.Error(),
				Code:    500,
			})
			result.Failure++
			return fmt.Errorf("import aborted at line %d: %v", line, err)
		}
		result.Success++
	}

	return nil
}

