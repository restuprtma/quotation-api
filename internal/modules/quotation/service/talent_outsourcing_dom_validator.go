package service

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type TalentOutsourcingIntegrityExpectations struct {
	Input       domain.TalentOutsourcingInput
	Calculation domain.TalentOutsourcingCalculation
	Profile     domain.ResolvedTalentOutsourcingProfile
}

func talentAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func talentNodes(n *html.Node, out *[]*html.Node) {
	if n.Type == html.ElementNode {
		*out = append(*out, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		talentNodes(c, out)
	}
}
func talentFieldValues(document string) (map[string]string, error) {
	root, e := html.Parse(strings.NewReader(document))
	if e != nil {
		return nil, e
	}
	var nodes []*html.Node
	talentNodes(root, &nodes)
	found := map[string]string{}
	for _, n := range nodes {
		if f := talentAttr(n, "data-field"); f != "" {
			if _, ok := found[f]; ok {
				return nil, fmt.Errorf("%w: duplicate data-field %s", ErrUnsafeHTML, f)
			}
			v := talentAttr(n, "data-value")
			if v == "" {
				return nil, fmt.Errorf("%w: data-field %s missing data-value", ErrUnsafeHTML, f)
			}
			found[f] = v
		}
	}
	return found, nil
}
func talentExpectedFields(in domain.TalentOutsourcingInput, c domain.TalentOutsourcingCalculation) map[string]int64 {
	m := map[string]int64{"personnel_subtotal": c.PersonnelSubtotal, "total_before_tax": c.TotalBeforeTax, "grand_total": c.GrandTotal, "payment_total": c.PaymentTotal}
	if c.NonPersonnelSubtotal > 0 {
		m["non_personnel_subtotal"] = c.NonPersonnelSubtotal
	}
	if c.DPPAmount > 0 {
		m["dpp_amount"] = c.DPPAmount
		m["tax_amount"] = c.TaxAmount
	}
	for gi, g := range c.CostGroups {
		for i, x := range g.Items {
			m[fmt.Sprintf("cost_groups.%d.items.%d.line_total", gi, i)] = x.LineTotal
		}
	}
	for i, x := range c.Preparation {
		m[fmt.Sprintf("preparation.%d.line_total", i)] = x.LineTotal
	}
	if c.PreparationSubtotal > 0 {
		m["preparation_subtotal"] = c.PreparationSubtotal
	}
	if c.PreparationDiscountAmount > 0 {
		m["preparation_discount_amount"] = c.PreparationDiscountAmount
	}
	for i, x := range c.Squad {
		m[fmt.Sprintf("squad.%d.line_total", i)] = x.LineTotal
	}
	if c.SquadSubtotal > 0 {
		m["squad_subtotal"] = c.SquadSubtotal
	}
	for i, x := range in.PaymentSchedule {
		m[fmt.Sprintf("payments.%d.nominal", i)] = x.Nominal
	}
	return m
}
func ValidateTalentOutsourcingIntegrity(document string, e TalentOutsourcingIntegrityExpectations) error {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return fmt.Errorf("%w: parse: %v", ErrUnsafeHTML, err)
	}
	var nodes []*html.Node
	talentNodes(root, &nodes)
	var article *html.Node
	sections := map[string]*html.Node{}
	limits := map[string]bool{}
	for _, n := range nodes {
		if n.Data == "script" || n.Data == "style" {
			return fmt.Errorf("%w: unsafe element", ErrUnsafeHTML)
		}
		if n.Data == "article" && talentAttr(n, "data-service") == "talent_outsourcing" {
			if article != nil {
				return fmt.Errorf("%w: duplicate talent root", ErrUnsafeHTML)
			}
			article = n
		}
		if code := talentAttr(n, "data-section-code"); code != "" {
			sections[code] = n
		}
		if c := talentAttr(n, "data-limitation-code"); c != "" {
			limits[c] = true
		}
	}
	if article == nil {
		return fmt.Errorf("%w: talent root missing", ErrUnsafeHTML)
	}
	for _, s := range e.Profile.Sections {
		n := sections[s.Code]
		if n == nil {
			return fmt.Errorf("%w: section %s missing", ErrUnsafeHTML, s.Code)
		}
		if !strings.Contains(nodeText(n), fmt.Sprintf("%d.", s.Number)) {
			return fmt.Errorf("%w: section %s number", ErrUnsafeHTML, s.Code)
		}
	}
	for _, code := range e.Input.LimitationCodes {
		if !limits[string(code)] {
			return fmt.Errorf("%w: limitation %s missing", ErrUnsafeHTML, code)
		}
	}
	got, err := talentFieldValues(document)
	if err != nil {
		return err
	}
	want := talentExpectedFields(e.Input, e.Calculation)
	if len(got) != len(want) {
		return fmt.Errorf("%w: numeric field count mismatch", ErrUnsafeHTML)
	}
	for f, v := range want {
		if got[f] != strconv.FormatInt(v, 10) {
			return fmt.Errorf("%w: %s mismatch", ErrUnsafeHTML, f)
		}
	}
	return nil
}
func CheckTalentOutsourcingCommercialModified(document string, in domain.TalentOutsourcingInput, c domain.TalentOutsourcingCalculation) (bool, error) {
	got, err := talentFieldValues(document)
	if err != nil {
		return false, err
	}
	want := talentExpectedFields(in, c)
	if len(got) != len(want) {
		return true, nil
	}
	for f, v := range want {
		if got[f] != strconv.FormatInt(v, 10) {
			return true, nil
		}
	}
	return false, nil
}
func CheckTalentOutsourcingLegalModified(document string, in domain.TalentOutsourcingInput, p domain.ResolvedTalentOutsourcingProfile) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return false, err
	}
	var nodes []*html.Node
	talentNodes(root, &nodes)
	seen := map[string]bool{}
	for _, n := range nodes {
		if c := talentAttr(n, "data-limitation-code"); c != "" {
			seen[c] = true
		}
	}
	for _, c := range in.LimitationCodes {
		if !seen[string(c)] {
			return true, nil
		}
	}
	return false, nil
}
