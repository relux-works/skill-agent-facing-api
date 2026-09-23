package agentquery

import (
	"crypto/sha256"
	"encoding/binary"
)

func DefaultQueryLimits() QueryLimits {
	return QueryLimits{
		MaxSourceBytes: 65536, MaxStatements: 16, MaxExpressionDepth: 16, MaxExpressionNodes: 128,
		MaxLiteralBytes: 4096, MaxRegexBytesPerStatement: 1024, MaxSetEntries: 100, MaxSortCriteria: 4,
		MaxSnapshotRows: 100000, MaxSnapshotTypedBytes: 67108864, MaxStoredScalarBytes: 65536,
		MaxStoredArrayEntries: 1024, MaxGroupCardinality: 10000, MaxSkip: 100000, MaxTake: 1000,
		MaxTopLevelResultsWithoutTake: 1000, MaxWorkUnits: 10000000, MaxResponseBytes: 4194304,
		ErrorFramingReserveBytes: 8192,
	}
}

func qmError(code, message string) error { return &Error{Code: code, Message: message} }

func normalizeQueryLimits(in QueryLimits) (QueryLimits, error) {
	d := DefaultQueryLimits()
	vals := []*uint64{&in.MaxSourceBytes, &in.MaxStatements, &in.MaxExpressionDepth, &in.MaxExpressionNodes, &in.MaxLiteralBytes, &in.MaxRegexBytesPerStatement, &in.MaxSetEntries, &in.MaxSortCriteria, &in.MaxSnapshotRows, &in.MaxSnapshotTypedBytes, &in.MaxStoredScalarBytes, &in.MaxStoredArrayEntries, &in.MaxGroupCardinality, &in.MaxSkip, &in.MaxTake, &in.MaxTopLevelResultsWithoutTake, &in.MaxWorkUnits, &in.MaxResponseBytes, &in.ErrorFramingReserveBytes}
	defs := []uint64{d.MaxSourceBytes, d.MaxStatements, d.MaxExpressionDepth, d.MaxExpressionNodes, d.MaxLiteralBytes, d.MaxRegexBytesPerStatement, d.MaxSetEntries, d.MaxSortCriteria, d.MaxSnapshotRows, d.MaxSnapshotTypedBytes, d.MaxStoredScalarBytes, d.MaxStoredArrayEntries, d.MaxGroupCardinality, d.MaxSkip, d.MaxTake, d.MaxTopLevelResultsWithoutTake, d.MaxWorkUnits, d.MaxResponseBytes, d.ErrorFramingReserveBytes}
	for i, p := range vals {
		if *p == 0 {
			*p = defs[i]
		} else if *p > defs[i] {
			return QueryLimits{}, qmError("predicate_limit", "query limit exceeds default")
		}
	}
	if in.MaxResponseBytes < 8192 || in.ErrorFramingReserveBytes < 480 {
		return QueryLimits{}, qmError("predicate_limit", "query limit is below supported minimum")
	}
	return in, nil
}

func queryLimitsDigest(l QueryLimits) [32]byte {
	b := make([]byte, 0, len("agentquery.composable-query.v1")+1+19*8)
	b = append(b, "agentquery.composable-query.v1"...)
	b = append(b, 0)
	vals := []uint64{l.MaxSourceBytes, l.MaxStatements, l.MaxExpressionDepth, l.MaxExpressionNodes, l.MaxLiteralBytes, l.MaxRegexBytesPerStatement, l.MaxSetEntries, l.MaxSortCriteria, l.MaxSnapshotRows, l.MaxSnapshotTypedBytes, l.MaxStoredScalarBytes, l.MaxStoredArrayEntries, l.MaxGroupCardinality, l.MaxSkip, l.MaxTake, l.MaxTopLevelResultsWithoutTake, l.MaxWorkUnits, l.MaxResponseBytes, l.ErrorFramingReserveBytes}
	var x [8]byte
	for _, v := range vals {
		binary.BigEndian.PutUint64(x[:], v)
		b = append(b, x[:]...)
	}
	return sha256.Sum256(b)
}

func (s *Schema[T]) SetQueryLimits(limits QueryLimits) error {
	if s.querySealed {
		return qmError("predicate_unsupported", "query model configuration is sealed")
	}
	n, err := normalizeQueryLimits(limits)
	if err != nil {
		return err
	}
	s.queryLimits = n
	return nil
}
func (s *Schema[T]) QueryLimits() QueryLimits { return s.queryLimits }
