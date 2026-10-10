// Package detect classifies typed samples without database or other I/O.
//
// It implements the frozen m1a-v1 Detector contract with immutable rules for
// email, URL, IP addresses (IPv4 and IPv6), UUID, Chinese calendar dates, and
// the China-first fields: resident identity card, mainland mobile, bank card,
// postal code, unified social credit identifier, bounded geographic names,
// bounded Han personal names, and Chinese address fragments. Format matches and
// column-context eligibility are evaluated separately: a context-required rule
// such as cn_city stays Unknown without a matching column name, and a phone
// column holding emails yields Unknown with ConflictingEvidence instead of a
// rule match. Insufficient, empty, or conflicting samples also stay Unknown:
// Unknown never means safe.
//
// The China-first rules are bounded format checks with documented, versioned
// rule data and deliberately limited geographic/historical coverage. They are
// not identity, registration, or entitlement verification. See cn.go.
package detect
