package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadCorpus(t *testing.T) corpus {
	t.Helper()
	var c corpus
	b, e := os.ReadFile(filepath.Join("..", "..", "internal", "modules", "quotation", "service", "testdata", "meeting_extraction_quality", "corpus.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func perfect(c corpus) predictionFile {
	p := predictionFile{QuotaMargin: 1}
	for _, r := range c.Records {
		fields := map[string]string{}
		for key, value := range r.Gold.Fields {
			fields[key] = value
		}
		p.Records = append(p.Records, prediction{ID: r.ID, Scheme: r.Gold.Scheme, Decision: r.Gold.Decision, Fields: fields, EvidenceLines: r.Gold.EvidenceLines, Calls: 4, Tokens: 100, LatencyMS: 500})
	}
	return p
}
func TestCorpusCoverageAndNoCustomerIdentifiers(t *testing.T) {
	c := loadCorpus(t)
	if len(c.Records) != 17 {
		t.Fatalf("records=%d", len(c.Records))
	}
	counts := map[string]int{}
	tags := map[string]bool{}
	for _, r := range c.Records {
		counts[r.Gold.Scheme]++
		for _, tag := range r.Tags {
			tags[tag] = true
		}
		if r.ID == "" || r.Gold.Scheme == "" || len(r.Gold.EvidenceLines) == 0 {
			t.Fatalf("invalid gold record: %+v", r)
		}
	}
	for _, scheme := range []string{"dedicated_squad", "subscription_saas", "white_label", "standalone_maintenance", "custom_development", "kiosk_queue_system", "talent_outsourcing", "cloud_infrastructure"} {
		if counts[scheme] < 2 {
			t.Fatalf("%s coverage=%d", scheme, counts[scheme])
		}
	}
	for _, tag := range []string{"messy", "bilingual", "multi_service", "injection"} {
		if !tags[tag] {
			t.Fatalf("missing %s", tag)
		}
	}
}
func TestEvaluatorPassesGoldAndFailsReleaseGates(t *testing.T) {
	c := loadCorpus(t)
	p := perfect(c)
	r, e := evaluate(c, p)
	if e != nil || !r.Pass || r.FieldF1 != 1 || r.SchemePrecision != 1 {
		t.Fatalf("gold should pass: %+v %v", r, e)
	}
	p.Records[0].Fields["duration_months"] = "99"
	p.SnapshotRegression = true
	p.QuotaMargin = 0
	r, e = evaluate(c, p)
	if e != nil || r.Pass || r.CriticalNumericFalsePositive != 1 {
		t.Fatalf("bad run must fail: %+v %v", r, e)
	}
}

func TestEvaluatorCountsUnsafeSupportAndProviderOutcomes(t *testing.T) {
	c := loadCorpus(t)
	p := perfect(c)
	p.Records[0].EvidenceLines = []int{99}
	p.Records[0].Status = "429"
	p.Records[1].Status = "504"
	r, err := evaluate(c, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.FalseSupport != 1 || r.EvidenceAccepted != 19 || r.RateLimited429 != 1 || r.GatewayTimeout504 != 1 {
		t.Fatalf("unsafe support/provider outcomes not aggregated: %+v", r)
	}
}
func TestRunEmitsAggregateOnlyReport(t *testing.T) {
	c := loadCorpus(t)
	p := perfect(c)
	dir := t.TempDir()
	path := filepath.Join(dir, "predictions.json")
	b, _ := json.Marshal(p)
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	var out, errOut bytes.Buffer
	corpusPath := filepath.Join("..", "..", "internal", "modules", "quotation", "service", "testdata", "meeting_extraction_quality", "corpus.json")
	if code := run([]string{"-corpus", corpusPath, "-predictions", path}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	for _, forbidden := range []string{"Aurora Co", "Northwind", "meeting_notes", "notes"} {
		if bytes.Contains(out.Bytes(), []byte(forbidden)) {
			t.Fatalf("report leaked %q: %s", forbidden, out.String())
		}
	}
}
