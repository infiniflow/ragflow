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

// Package transaction holds business transactions that span DAO calls: it
// composes several DAO reads and writes into one atomic unit of work, and owns
// the business rules that decide what those reads and writes should be.
//
// Short circuits belong here too. Deciding that a request can match nothing is
// a business rule, and resolving it before composing the query means no DAO
// call — and therefore no transaction — is opened for work that has a known
// answer.
package transaction

// DocumentQueryCircuit decides whether a document query leaves the process at
// all. A document request can ask for an intersection of IDs that business
// rules already know is empty, and no amount of database work changes that
// answer.
//
// The document ID clause only participates when docIDFilterApplied is true, so
// "no IDs" is ambiguous on its own: with it false the caller never asked for
// the clause, with it true the caller asked for an intersection that came out
// empty. Only the second case opens the circuit.
type DocumentQueryCircuit struct {
	docIDFilterApplied bool
	docIDs             []string
}

// NewDocumentQueryCircuit builds the circuit for one document query.
func NewDocumentQueryCircuit(docIDFilterApplied bool, docIDs []string) DocumentQueryCircuit {
	return DocumentQueryCircuit{docIDFilterApplied: docIDFilterApplied, docIDs: docIDs}
}

// IsCircuitOpen reports whether the query can match no documents, letting the
// caller answer from business rules instead of opening a database round trip.
func (c DocumentQueryCircuit) IsCircuitOpen() bool {
	return c.docIDFilterApplied && len(c.docIDs) == 0
}

// CircuitOpenFilters is the aggregate breakdown a document filter query reports
// once the circuit is open. The bucket types mirror the ones a query that ran
// would build, because callers type-assert on them without checking.
func (c DocumentQueryCircuit) CircuitOpenFilters() map[string]interface{} {
	return map[string]interface{}{
		"suffix":           map[string]int64{},
		"ingestion_status": map[string]int64{},
		"metadata":         map[string]interface{}{},
	}
}
