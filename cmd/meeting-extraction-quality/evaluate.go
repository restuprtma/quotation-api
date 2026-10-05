// Non-production, aggregate-only release-gate evaluator.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

const corpusSchema = "meeting_extraction_quality_v1"

type corpus struct {
	SchemaVersion string       `json:"schema_version"`
	Records       []goldRecord `json:"records"`
}
type goldRecord struct {
	ID   string   `json:"id"`
	Gold gold     `json:"gold"`
	Tags []string `json:"tags"`
}
type gold struct {
	Scheme, Decision string
	Fields           map[string]string `json:"fields"`
	EvidenceLines    []int             `json:"evidence_lines"`
}
type predictionFile struct {
	SnapshotRegression bool         `json:"snapshot_regression"`
	QuotaMargin        int64        `json:"quota_margin"`
	Records            []prediction `json:"records"`
}
type prediction struct {
	ID            string            `json:"id"`
	Scheme        string            `json:"scheme"`
	Decision      string            `json:"decision"`
	Fields        map[string]string `json:"fields"`
	EvidenceLines []int             `json:"evidence_lines"`
	Calls         int64             `json:"calls"`
	Tokens        int64             `json:"tokens"`
	LatencyMS     int64             `json:"latency_ms"`
	Status        string            `json:"status"`
}
type count struct {
	TruePositive  int `json:"true_positive"`
	FalsePositive int `json:"false_positive"`
	FalseNegative int `json:"false_negative"`
}

func (c count) Precision() float64 {
	if c.TruePositive+c.FalsePositive == 0 {
		return 1
	}
	return float64(c.TruePositive) / float64(c.TruePositive+c.FalsePositive)
}
func (c count) Recall() float64 {
	if c.TruePositive+c.FalseNegative == 0 {
		return 1
	}
	return float64(c.TruePositive) / float64(c.TruePositive+c.FalseNegative)
}
func (c count) F1() float64 {
	p, r := c.Precision(), c.Recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

type report struct {
	Records                      int      `json:"records"`
	SchemePrecision              float64  `json:"scheme_precision"`
	SchemeCorrect                int      `json:"scheme_correct"`
	SchemePredicted              int      `json:"scheme_predicted"`
	AmbiguousNeedsReview         int      `json:"ambiguous_needs_review"`
	Fields                       count    `json:"fields"`
	FieldF1                      float64  `json:"field_f1"`
	EvidenceAccepted             int      `json:"evidence_accepted"`
	EvidenceSubmitted            int      `json:"evidence_submitted"`
	FalseSupport                 int      `json:"false_support"`
	CriticalNumericFalsePositive int      `json:"critical_numeric_false_positive"`
	Repairs                      int      `json:"repairs"`
	CallsP50                     int64    `json:"calls_p50"`
	CallsP95                     int64    `json:"calls_p95"`
	TokensTotal                  int64    `json:"tokens_total"`
	LatencyP50MS                 int64    `json:"latency_p50_ms"`
	LatencyP95MS                 int64    `json:"latency_p95_ms"`
	RateLimited429               int      `json:"rate_limited_429"`
	GatewayTimeout504            int      `json:"gateway_timeout_504"`
	QuotaMargin                  int64    `json:"quota_margin"`
	SnapshotRegression           bool     `json:"snapshot_regression"`
	Pass                         bool     `json:"pass"`
	Failures                     []string `json:"failures"`
}

func evaluate(c corpus, p predictionFile) (report, error) {
	if c.SchemaVersion != corpusSchema || len(c.Records) < 17 {
		return report{}, errors.New("invalid corpus")
	}
	byID := map[string]prediction{}
	for _, item := range p.Records {
		if item.ID == "" {
			return report{}, errors.New("prediction missing id")
		}
		if _, ok := byID[item.ID]; ok {
			return report{}, fmt.Errorf("duplicate prediction %q", item.ID)
		}
		byID[item.ID] = item
	}
	r := report{Records: len(c.Records), QuotaMargin: p.QuotaMargin, SnapshotRegression: p.SnapshotRegression}
	calls := []int64{}
	latency := []int64{}
	for _, rec := range c.Records {
		item, ok := byID[rec.ID]
		if !ok {
			return report{}, fmt.Errorf("missing prediction %q", rec.ID)
		}
		calls = append(calls, item.Calls)
		latency = append(latency, item.LatencyMS)
		r.TokensTotal += item.Tokens
		if item.Status == "repair" {
			r.Repairs++
		}
		if item.Status == "429" {
			r.RateLimited429++
		}
		if item.Status == "504" {
			r.GatewayTimeout504++
		}
		if rec.Gold.Scheme == "multi_service" {
			if item.Decision == "needs_review" {
				r.AmbiguousNeedsReview++
			}
		} else if item.Decision != "needs_review" {
			r.SchemePredicted++
			if item.Scheme == rec.Gold.Scheme {
				r.SchemeCorrect++
			}
		}
		validEvidence := lineSet(rec.Gold.EvidenceLines)
		badEvidence := false
		for _, line := range item.EvidenceLines {
			r.EvidenceSubmitted++
			if validEvidence[line] {
				r.EvidenceAccepted++
			} else {
				badEvidence = true
			}
		}
		if badEvidence {
			r.FalseSupport++
		}
		for key, actual := range item.Fields {
			expected, wanted := rec.Gold.Fields[key]
			if wanted && actual == expected {
				r.Fields.TruePositive++
			} else {
				r.Fields.FalsePositive++
				if isCriticalNumeric(key, actual) {
					r.CriticalNumericFalsePositive++
				}
			}
		}
		for key := range rec.Gold.Fields {
			if item.Fields[key] != rec.Gold.Fields[key] {
				r.Fields.FalseNegative++
			}
		}
	}
	r.SchemePrecision = ratio(r.SchemeCorrect, r.SchemePredicted)
	r.FieldF1 = r.Fields.F1()
	r.CallsP50 = percentile(calls, 50)
	r.CallsP95 = percentile(calls, 95)
	r.LatencyP50MS = percentile(latency, 50)
	r.LatencyP95MS = percentile(latency, 95)
	if r.CriticalNumericFalsePositive != 0 {
		r.Failures = append(r.Failures, "critical_numeric_false_positive")
	}
	if r.SchemePrecision < .95 {
		r.Failures = append(r.Failures, "scheme_precision_below_95")
	}
	if r.QuotaMargin <= 0 {
		r.Failures = append(r.Failures, "quota_margin_not_positive")
	}
	if r.SnapshotRegression {
		r.Failures = append(r.Failures, "snapshot_regression")
	}
	r.Pass = len(r.Failures) == 0
	return r, nil
}
func lineSet(lines []int) map[int]bool {
	out := map[int]bool{}
	for _, v := range lines {
		out[v] = true
	}
	return out
}
func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}
func percentile(values []int64, p int) int64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]int64(nil), values...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	idx := int(math.Ceil(float64(p)/100*float64(len(v)))) - 1
	if idx < 0 {
		idx = 0
	}
	return v[idx]
}
func isCriticalNumeric(key, value string) bool {
	key = strings.ToLower(key)
	if key != "duration_months" && key != "team_size" && key != "headcount" && key != "kiosk_count" && key != "outlet_count" && key != "user_count" {
		return false
	}
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
func marshalReport(r report) ([]byte, error) { return json.MarshalIndent(r, "", "  ") }
