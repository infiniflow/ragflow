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

package models

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Aliyun Cloud Open API V3 ("ACS3-HMAC-SHA256") request signer.
//
// Reference: https://www.alibabacloud.com/help/en/sdk/product-overview/v3-request-structure-and-signature
//
// V3 is the Cloud-wide control-plane signing scheme — same algorithm used by
// every Aliyun product (ECS, OSS, RAM, BSS, ...). It authenticates with an
// AccessKey ID + Secret pair, NOT a DashScope sk-... Bearer token: the two
// credential systems live side by side on every Aliyun account but are not
// interchangeable, so callers that don't have an AccessKey pair must reject
// the request before reaching here.
//
// The signer is deliberately stateless and takes nonce + timestamp as
// parameters rather than generating them internally so its canonical-request
// and signature outputs are byte-deterministic under test. In production the
// caller (see signBssRequest below) generates a random nonce + UTC timestamp
// per request.

const (
	aliyunSigAlgorithm    = "ACS3-HMAC-SHA256"
	aliyunSigHeaderPrefix = "x-acs-"
)

// aliyunPercentEncode URI-encodes a single string per RFC 3986 strict rules:
//
//	keep:    A-Z a-z 0-9 - _ . ~
//	encode:  everything else as %XX (uppercase hex)
//
// Go's url.QueryEscape encodes space as '+' (form encoding) and leaves '*'
// and '~' unencoded relative to AWS-style strict rules, so this helper fixes
// those three deltas. The result matches what Aliyun's official SDKs emit.
func aliyunPercentEncode(s string) string {
	enc := url.QueryEscape(s)
	enc = strings.ReplaceAll(enc, "+", "%20")
	enc = strings.ReplaceAll(enc, "*", "%2A")
	enc = strings.ReplaceAll(enc, "%7E", "~")
	return enc
}

// aliyunCanonicalQueryString returns the V3-canonical encoding of a query map:
// every key+value individually percent-encoded, then sorted lexicographically
// by encoded key, then joined as "k=v&k=v".
//
// Multi-value entries are emitted as separate "k=v" pairs in input order; the
// sort step then interleaves them with other keys. Empty value strings keep
// their "k=" form so the canonical form survives intentionally-empty params.
func aliyunCanonicalQueryString(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(query))
	for _, k := range keys {
		ek := aliyunPercentEncode(k)
		vals := query[k]
		if len(vals) == 0 {
			parts = append(parts, ek+"=")
			continue
		}
		for _, v := range vals {
			parts = append(parts, ek+"="+aliyunPercentEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

// aliyunCanonicalHeaders returns the canonical header block plus the
// semicolon-joined list of signed header names per V3.
//
// Only headers required by the V3 scheme participate: "host" + every header
// whose name starts with "x-acs-". This matches Aliyun's reference signer
// (the SDK's HmacSha256Signer) and excludes ad-hoc transport headers
// (User-Agent, Content-Type, Accept, ...) so those can be added/removed by
// intermediaries without invalidating the signature.
//
// Header names are lowercased; header values are trimmed of leading/trailing
// whitespace. The canonical block ends with a trailing "\n" per spec.
func aliyunCanonicalHeaders(h http.Header) (canonical, signed string) {
	type kv struct{ k, v string }
	pairs := make([]kv, 0, len(h))
	for name, vs := range h {
		lower := strings.ToLower(name)
		if lower != "host" && !strings.HasPrefix(lower, aliyunSigHeaderPrefix) {
			continue
		}
		if len(vs) == 0 {
			continue
		}
		pairs = append(pairs, kv{k: lower, v: strings.TrimSpace(vs[0])})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })

	var canonBuf strings.Builder
	names := make([]string, 0, len(pairs))
	for _, p := range pairs {
		canonBuf.WriteString(p.k)
		canonBuf.WriteString(":")
		canonBuf.WriteString(p.v)
		canonBuf.WriteString("\n")
		names = append(names, p.k)
	}
	return canonBuf.String(), strings.Join(names, ";")
}

// aliyunHexSHA256 returns the lowercase hex SHA-256 digest of body. Used
// for the X-Acs-Content-Sha256 header and the hashed-payload slot in the
// canonical request.
func aliyunHexSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// aliyunCanonicalRequest assembles the six-line block whose SHA-256 digest
// feeds into the string-to-sign. The trailing "\n" rules match V3 exactly:
// every line uses "\n" as separator, the canonical headers block already
// ends with "\n", and the line after signed headers is the hashed payload
// with NO trailing newline.
func aliyunCanonicalRequest(method, uri, canonicalQuery, canonicalHeaders, signedHeaders, hashedPayload string) string {
	return method + "\n" +
		uri + "\n" +
		canonicalQuery + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		hashedPayload
}

// aliyunStringToSign:  ACS3-HMAC-SHA256\nhex(sha256(canonicalRequest))
func aliyunStringToSign(canonicalRequest string) string {
	return aliyunSigAlgorithm + "\n" + aliyunHexSHA256([]byte(canonicalRequest))
}

// aliyunSign returns lowercase-hex HMAC-SHA256(secret, stringToSign).
func aliyunSign(secret, stringToSign string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return hex.EncodeToString(mac.Sum(nil))
}

// aliyunAuthorizationHeader assembles the Authorization header value per V3.
//
// Format:
//
//	ACS3-HMAC-SHA256 Credential=<AccessKeyId>,SignedHeaders=<h1;h2>,Signature=<hex>
//
// The single space after the algorithm and the comma-without-space separators
// are required by Aliyun's parser.
func aliyunAuthorizationHeader(accessKeyID, signedHeaders, signature string) string {
	return fmt.Sprintf("%s Credential=%s,SignedHeaders=%s,Signature=%s",
		aliyunSigAlgorithm, accessKeyID, signedHeaders, signature)
}

// signAliyunV3 stamps an http.Request with all V3 mandatory headers
// (x-acs-action, x-acs-version, x-acs-date, x-acs-signature-nonce,
// x-acs-content-sha256, host) and the Authorization header carrying the
// HMAC-SHA256 signature.
//
// Caller responsibilities:
//   - req.URL.Host must be set (e.g. "business.aliyuncs.com")
//   - payload is the exact request body bytes (nil/empty allowed for GET)
//   - timestamp is an ISO-8601 UTC string ("2006-01-02T15:04:05Z")
//   - nonce is a unique random string per request (caller-supplied so tests
//     can pin it)
//
// The request body is NOT consumed; callers must have already constructed
// req with that body. The signer does not mutate req.URL.
//
// Errors are returned only for malformed input (missing host). Cryptographic
// failures are not possible with hmac/sha256 from the stdlib.
func signAliyunV3(req *http.Request, accessKeyID, accessKeySecret, action, version, nonce, timestamp string, payload []byte) error {
	if req == nil || req.URL == nil || req.URL.Host == "" {
		return fmt.Errorf("aliyun sigv3: request has no host")
	}
	if accessKeyID == "" || accessKeySecret == "" {
		return fmt.Errorf("aliyun sigv3: missing access key credentials")
	}

	hashedPayload := aliyunHexSHA256(payload)

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("x-acs-action", action)
	req.Header.Set("x-acs-version", version)
	req.Header.Set("x-acs-date", timestamp)
	req.Header.Set("x-acs-signature-nonce", nonce)
	req.Header.Set("x-acs-content-sha256", hashedPayload)

	canonicalQuery := aliyunCanonicalQueryString(req.URL.Query())
	canonicalHeaders, signedHeaders := aliyunCanonicalHeaders(req.Header)

	uri := req.URL.EscapedPath()
	if uri == "" {
		uri = "/"
	}

	canonicalRequest := aliyunCanonicalRequest(
		strings.ToUpper(req.Method),
		uri,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	)
	stringToSign := aliyunStringToSign(canonicalRequest)
	signature := aliyunSign(accessKeySecret, stringToSign)

	req.Header.Set("Authorization", aliyunAuthorizationHeader(accessKeyID, signedHeaders, signature))
	return nil
}
