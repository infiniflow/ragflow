//go:build !cgo

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package component

import "ragflow/internal/ingestion/component/schema"

func maybeResolveAutoPDFLayout(setup schema.ParserSetup, filename string, binary []byte) (schema.ParserSetup, error) {
	return setup, nil
}
