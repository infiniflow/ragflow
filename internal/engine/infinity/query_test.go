//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package infinity

import (
	"encoding/binary"
	"testing"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

// TestQueryResult tests the Infinity ToResult() method directly
func TestQueryResult(t *testing.T) {
	// Connect directly to Infinity
	conn, err := infinity.Connect(infinity.NetworkAddress{IP: "127.0.0.1", Port: 23817})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Disconnect()

	// Get database
	dbName := "default_db"
	db, err := conn.GetDatabase(dbName)
	if err != nil {
		t.Fatalf("Failed to get database: %v", err)
	}

	// Get the metadata table
	tableName := "ragflow_doc_meta_c33a863733ec11f1957984ba59049aa3"
	table, err := db.GetTable(tableName)
	if err != nil {
		t.Fatalf("Failed to get table %s: %v", tableName, err)
	}

	// Query with just id and meta_fields columns
	t.Log("Querying with Output([]string{\"id\", \"kb_id\", \"meta_fields\"})...")
	queryTable := table.Output([]string{"id", "kb_id", "meta_fields"}).Limit(10).Offset(0)

	// Call ToResult() directly
	result, err := queryTable.ToResult()
	if err != nil {
		t.Fatalf("ToResult() failed: %v", err)
	}

	// Check if it's a QueryResult
	qr, ok := result.(*infinity.QueryResult)
	if !ok {
		t.Fatalf("Expected *infinity.QueryResult, got %T", result)
	}

	// Print all data
	t.Logf("QueryResult.Data keys: %v", getMapKeys(qr.Data))
	for colName, colData := range qr.Data {
		t.Logf("Column '%s' length: %d", colName, len(colData))
		for i, val := range colData {
			t.Logf("  [%d] type=%T value=%v", i, val, val)
		}
	}

	// Parse the length-prefixed meta_fields to show what the SDK returns
	if metaFieldsData, exists := qr.Data["meta_fields"]; exists && len(metaFieldsData) > 0 {
		t.Log("\n--- Parsing length-prefixed meta_fields data ---")
		rawData := metaFieldsData[0].([]byte)
		t.Logf("Raw meta_fields bytes: %v", rawData)
		t.Logf("Raw meta_fields hex: %x", rawData)

		// Parse like Infinity SDK stores it: [4-byte length][JSON][4-byte length][JSON]...
		offset := 0
		rowIndex := 0
		for offset < len(rawData) {
			if offset+4 > len(rawData) {
				t.Logf("Row %d: not enough bytes for length prefix", rowIndex)
				break
			}
			// Read 4-byte length (little-endian)
			length := binary.LittleEndian.Uint32(rawData[offset : offset+4])
			t.Logf("Row %d: length prefix = %d bytes", rowIndex, length)

			if offset+4+int(length) > len(rawData) {
				t.Logf("Row %d: data would exceed buffer, stopping", rowIndex)
				break
			}

			jsonBytes := rawData[offset+4 : offset+4+int(length)]
			t.Logf("Row %d: JSON bytes = %s", rowIndex, string(jsonBytes))

			offset += 4 + int(length)
			rowIndex++
		}
	}
}

func getMapKeys(m map[string][]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
